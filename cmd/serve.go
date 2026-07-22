package cmd

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-corekit/logkit"
	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/consolidation"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/danieljustus/symaira-memory/internal/mcp"
	"github.com/danieljustus/symaira-memory/internal/security"
	"github.com/danieljustus/symaira-memory/internal/working"
	"github.com/spf13/cobra"
)

var (
	servePort     int
	serveProfile  string
	serveLogLevel string
)

func init() {
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 0, "Port to listen on for HTTP REST API mode (default stdio)")
	serveCmd.Flags().StringVar(&serveProfile, "profile", "", "Agent profile name to enforce (env: SYMMEMORY_PROFILE)")
	serveCmd.Flags().StringVar(&serveLogLevel, "log-level", "", "Log level override: debug, info, warn, error (default from SYMMEMORY_LOG_LEVEL env)")
	rootCmd.AddCommand(serveCmd)
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Model Context Protocol (MCP) stdio server or HTTP API daemon",
	Long: `Starts the stdio transport JSON-RPC 2.0 server (default) or runs a local HTTP REST API 
server if a port is provided. This HTTP API daemon powers the browser extension.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if serveLogLevel != "" {
			var level slog.Level
			if err := level.UnmarshalText([]byte(serveLogLevel)); err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitConfig, exitcodes.KindConfig, "invalid log level %q (use debug, info, warn, or error)", serveLogLevel)
			}
			slog.SetDefault(logkit.New(os.Stderr, level, "text"))
		}

		cfg := GetConfig()

		profileName := serveProfile
		if profileName == "" {
			profileName = os.Getenv("SYMMEMORY_PROFILE")
		}

		var profile *db.Profile
		if profileName != "" {
			p, err := GetDB().GetProfileByName(profileName)
			if err != nil {
				slog.Error("Failed to look up profile", "profile", profileName, "error", err)
				return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to look up profile %q", profileName)
			}
			if p == nil {
				slog.Error("Unknown profile", "profile", profileName)
				return exitcodes.Wrapf(nil, exitcodes.ExitNotFound, exitcodes.KindNotFound, "unknown profile: %q", profileName)
			}
			profile = p
			slog.Info("Active profile", "name", profile.Name, "role", profile.Role)
		}

		jwtProvider, err := security.NewJWTProvider(cfg, GetDB())
		if err != nil {
			slog.Error("Failed to initialize JWT provider", "error", err)
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to initialize JWT provider")
		}
		server := mcp.NewServer(GetDB(), jwtProvider, Version, cfg)
		server.SetProfile(profile)

		if cfg != nil && cfg.Security.PIIEnabled != nil {
			server.SetPIIEnabled(*cfg.Security.PIIEnabled)
		}
		if servePort > 0 {
			if cfg.WorkingMemory.TTL != "" {
				go runBackgroundCompaction(cfg, GetDB())
			}
			if err := server.StartHTTPServer(servePort); err != nil {
				if errors.Is(err, http.ErrServerClosed) {
					return nil
				}
				slog.Error("HTTP server error", "error", err)
				return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "HTTP server error")
			}
			return nil
		} else {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			mcpSrv := server.MCPServer()
			errCh := make(chan error, 1)
			go func() {
				errCh <- mcpSrv.ServeStdio(ctx)
			}()

			select {
			case err := <-errCh:
				if err != nil {
					slog.Error("MCP server error", "error", err)
					return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "MCP server error")
				}
				return nil
			case <-ctx.Done():
				return nil
			}
		}
	},
}

func runBackgroundCompaction(cfg *config.Config, database *db.DB) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	embeddings := extractor.NewEmbeddingsGenerator(cfg)
	piiEnabled := cfg.Security.PIIEnabled != nil && *cfg.Security.PIIEnabled
	engine := consolidation.NewEngine(database, embeddings, cfg.Consolidation.URL, cfg.Consolidation.Model, cfg.Consolidation.Provider, piiEnabled)
	evictor := working.NewEvictor(database, embeddings, engine, piiEnabled)

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		result, err := evictor.CompactWorkingMemories(ctx, false)
		cancel()
		if err != nil {
			slog.Error("background compaction failed", "error", err)
			continue
		}
		if result.EvictedCount > 0 {
			slog.Info("background compaction completed", "expired", result.ExpiredCount, "evicted", result.EvictedCount)
		}
	}
}

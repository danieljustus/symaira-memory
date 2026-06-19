package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/mcp"
	"github.com/danieljustus/symaira-memory/internal/security"
	"github.com/spf13/cobra"
)

var (
	servePort    int
	serveProfile string
)

func init() {
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 0, "Port to listen on for HTTP REST API mode (default stdio)")
	serveCmd.Flags().StringVar(&serveProfile, "profile", "", "Agent profile name to enforce (env: SYMMEMORY_PROFILE)")
	rootCmd.AddCommand(serveCmd)
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Model Context Protocol (MCP) stdio server or HTTP API daemon",
	Long: `Starts the stdio transport JSON-RPC 2.0 server (default) or runs a local HTTP REST API 
server if a port is provided. This HTTP API daemon powers the browser extension.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg := GetConfig()

		profileName := serveProfile
		if profileName == "" {
			profileName = os.Getenv("SYMMEMORY_PROFILE")
		}

		var profile *db.Profile
		if profileName != "" {
			p, err := GetDB().GetProfileByName(profileName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to look up profile %q: %v\n", profileName, err)
				os.Exit(1)
			}
			if p == nil {
				fmt.Fprintf(os.Stderr, "Unknown profile: %q\n", profileName)
				os.Exit(1)
			}
			profile = p
			fmt.Fprintf(os.Stderr, "Active profile: %s (role=%s)\n", profile.Name, profile.Role)
		}

		jwtProvider, err := security.NewJWTProvider(cfg, GetDB())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize JWT provider: %v\n", err)
			os.Exit(1)
		}
		server := mcp.NewServer(GetDB(), jwtProvider, Version, cfg)
		server.SetProfile(profile)

		if cfg != nil && cfg.Security.PIIEnabled != nil {
			server.SetPIIEnabled(*cfg.Security.PIIEnabled)
		}
		if servePort > 0 {
			_ = server.StartHTTPServer(servePort)
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
					fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
					os.Exit(1)
				}
				os.Exit(0)
			case <-ctx.Done():
				os.Exit(0)
			}
		}
	},
}

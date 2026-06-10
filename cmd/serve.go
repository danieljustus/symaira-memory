package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/danieljustus/symaira-memory/internal/mcp"
	"github.com/danieljustus/symaira-memory/internal/security"
)

var (
	servePort int
)

func init() {
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 0, "Port to listen on for HTTP REST API mode (default stdio)")
	rootCmd.AddCommand(serveCmd)
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Model Context Protocol (MCP) stdio server or HTTP API daemon",
	Long: `Starts the stdio transport JSON-RPC 2.0 server (default) or runs a local HTTP REST API 
server if a port is provided. This HTTP API daemon powers the browser extension.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg := GetConfig()
		jwtProvider, err := security.NewJWTProvider(cfg, GetDB())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize JWT provider: %v\n", err)
			os.Exit(1)
		}
		server := mcp.NewServer(GetDB(), jwtProvider)

		if cfg != nil {
			server.SetPIIEnabled(cfg.Security.PIIEnabled)
		}
		if servePort > 0 {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			go func() {
				<-ctx.Done()
				os.Exit(0)
			}()
			_ = server.StartHTTPServer(servePort)
		} else {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			errCh := make(chan error, 1)
			go func() {
				errCh <- server.Serve(ctx)
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

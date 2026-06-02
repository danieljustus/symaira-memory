package cmd

import (
	"fmt"
	"os"

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
		jwtProvider, err := security.NewJWTProvider("")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize JWT provider: %v\n", err)
			os.Exit(1)
		}
		server := mcp.NewServer(RootDB, jwtProvider)
		if servePort > 0 {
			_ = server.StartHTTPServer(servePort)
		} else {
			server.Serve()
		}
	},
}

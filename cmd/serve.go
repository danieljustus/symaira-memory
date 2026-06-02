package cmd

import (
	"github.com/spf13/cobra"
	"github.com/danieljustus/symaira-memory/internal/mcp"
)

func init() {
	rootCmd.AddCommand(serveCmd)
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Model Context Protocol (MCP) server over stdio transport",
	Long: `Starts the stdio transport JSON-RPC 2.0 server. 
Enables advanced tools like memory_get, memory_set, memory_search, and memory_list 
for host clients like Claude Desktop, Cursor, and VS Code extensions.`,
	Run: func(cmd *cobra.Command, args []string) {
		server := mcp.NewServer(RootDB)
		server.Serve()
	},
}

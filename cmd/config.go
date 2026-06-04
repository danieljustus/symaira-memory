package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(configCmd)
}

var configCmd = &cobra.Command{
	Use:   "mcp-config",
	Short: "Print ready-to-use Model Context Protocol (MCP) configuration template",
	Long: `Prints standard JSON configuration blocks for integrating the symmemory stdio 
server into major MCP clients like Claude Desktop, Cursor, or VS Code Cline extension.`,
	Run: func(cmd *cobra.Command, args []string) {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting home dir: %v\n", err)
			os.Exit(1)
		}

		// Find the path to the current executable or assume symmemory standard path
		execPath, err := os.Executable()
		if err != nil {
			execPath = "symmemory" // fallback
		}

		// Build standard host config object
		config := map[string]interface{}{
			"mcpServers": map[string]interface{}{
				"symaira-memory": map[string]interface{}{
					"command": execPath,
					"args":    []string{"serve"},
				},
			},
		}

		configBytes, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to encode config: %v\n", err)
			os.Exit(1)
		}

		fmt.Fprintln(os.Stderr, "⚡ Copy and paste this block into your host configuration file:")
		fmt.Fprintf(os.Stderr, "\n📂 Claude Desktop Config Path:\n  %s\n", filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json"))
		fmt.Fprintf(os.Stderr, "\n🛠️ Cursor settings:\n  Cursor -> Settings -> Features -> MCP -> Click '+ Add New MCP Server'\n  Name: symaira-memory\n  Type: stdio\n  Command: %s serve\n", execPath)
		fmt.Fprintln(os.Stderr, "\n========================= CONFIGURATION BLOCK =========================")
		fmt.Fprintln(os.Stderr, string(configBytes))
		fmt.Fprintln(os.Stderr, "=======================================================================")
	},
}

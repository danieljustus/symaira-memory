package cmd

import (
	"fmt"
	"os"

	"github.com/danieljustus/symaira-memory/internal/importer"
	"github.com/spf13/cobra"
)

var (
	importTool   string
	importAll    bool
	importDryRun bool
)

func init() {
	importSessionsCmd.Flags().StringVar(&importTool, "tool", "", "Import from specific tool (e.g., claude-code)")
	importSessionsCmd.Flags().BoolVar(&importAll, "all", false, "Import from all registered tools")
	importSessionsCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "Show what would be imported without writing")
	rootCmd.AddCommand(importSessionsCmd)
}

var importSessionsCmd = &cobra.Command{
	Use:   "import",
	Short: "Import sessions from external AI coding tools",
	Long: `Import session data from external AI coding tools and convert them into
Symaira Memory facts. Currently supported tools: claude-code, codex, hermes, aider.

Examples:
  symmemory import --tool claude-code
  symmemory import --all
  symmemory import --tool claude-code --dry-run`,
	Run: func(cmd *cobra.Command, args []string) {
		if !importAll && importTool == "" {
			fmt.Fprintln(os.Stderr, "Error: either --tool or --all flag is required")
			os.Exit(1)
		}

		registry := importer.NewRegistry(GetDB())

		// TODO: Register actual importers here as they are implemented
		// For now, this is the framework for future importers

		tools := []string{}
		if importAll {
			tools = registry.List()
		} else {
			tools = []string{importTool}
		}

		results, err := registry.RunImport(cmd.Context(), tools, importDryRun)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Import failed: %v\n", err)
			os.Exit(1)
		}

		for _, result := range results {
			fmt.Printf("Tool: %s\n", result.Tool)
			fmt.Printf("  Sessions: %d\n", result.Sessions)
			fmt.Printf("  Facts: %d\n", result.Facts)
			fmt.Printf("  Skipped: %d\n", result.Skipped)
			fmt.Printf("  Errors: %d\n", result.Errors)
			if result.DryRun {
				fmt.Println("  (dry run - no changes made)")
			}
			fmt.Println()
		}
	},
}

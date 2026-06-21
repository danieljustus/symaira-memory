package cmd

import (
	"fmt"
	"os"

	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/danieljustus/symaira-memory/internal/importer"
	"github.com/danieljustus/symaira-memory/internal/importer/aider"
	"github.com/danieljustus/symaira-memory/internal/importer/calendar"
	"github.com/danieljustus/symaira-memory/internal/importer/claudecode"
	"github.com/danieljustus/symaira-memory/internal/importer/codex"
	"github.com/danieljustus/symaira-memory/internal/importer/email"
	"github.com/danieljustus/symaira-memory/internal/importer/git"
	"github.com/danieljustus/symaira-memory/internal/importer/github"
	"github.com/danieljustus/symaira-memory/internal/importer/hermes"
	"github.com/danieljustus/symaira-memory/internal/importer/obsidian"
	"github.com/danieljustus/symaira-memory/internal/importer/paperless"
	"github.com/danieljustus/symaira-memory/internal/importer/shellhistory"
	"github.com/spf13/cobra"
)

var (
	importTool   string
	importAll    bool
	importDryRun bool
	importList   bool
)

func init() {
	importSessionsCmd.Flags().StringVar(&importTool, "tool", "", "Import from specific tool (e.g., claude-code)")
	importSessionsCmd.Flags().BoolVar(&importAll, "all", false, "Import from all registered tools")
	importSessionsCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "Show what would be imported without writing")
	importSessionsCmd.Flags().BoolVar(&importList, "list", false, "Show configured importers and their status")
	rootCmd.AddCommand(importSessionsCmd)
}

var importSessionsCmd = &cobra.Command{
	Use:   "import",
	Short: "Import sessions from external AI coding tools",
	Long: `Import session data from external AI coding tools and convert them into
Symaira Memory facts.

Supported tools: claude-code, codex, hermes, aider, git, github, shell-history,
calendar, email, obsidian, paperless.

Examples:
  symmemory import --tool claude-code
  symmemory import --all
  symmemory import --tool claude-code --dry-run`,
	Run: func(cmd *cobra.Command, args []string) {
		if importList {
			cfg := GetConfig()
			tools := []string{"claude-code", "codex", "hermes", "aider", "git", "github", "shell-history", "calendar", "email", "obsidian", "paperless"}
			for _, tool := range tools {
				tc, ok := cfg.Import.Tools[tool]
				status := "not configured"
				if ok && tc.Path != "" {
					status = "configured (path: " + tc.Path + ")"
				} else if ok {
					status = "configured (no path)"
				}
				fmt.Printf("%-15s %s\n", tool, status)
			}
			return
		}

		if !importAll && importTool == "" {
			fmt.Fprintln(os.Stderr, "Error: either --tool, --all, or --list flag is required")
			os.Exit(1)
		}

		registry := importer.NewRegistry(GetDB(), extractor.NewEmbeddingsGenerator(GetConfig()))

		// Session importers
		registry.Register(claudecode.NewClaudeCodeImporter(""))
		registry.Register(codex.NewCodexImporter(""))
		registry.Register(hermes.NewHermesImporter(""))
		registry.Register(aider.NewAiderImporter(nil))

		// Data source importers
		registry.Register(git.NewGitImporter("", ""))
		registry.Register(github.NewGitHubImporter("", "", ""))
		registry.Register(shellhistory.NewShellHistoryImporter("", false, nil))
		registry.Register(calendar.NewCalendarImporter("", "", false, 7))
		registry.Register(email.NewEmailImporter("", "", 0))
		registry.Register(obsidian.NewObsidianImporter("", "", nil, nil, nil))
		registry.Register(paperless.NewPaperlessImporter("", "", "", "", 0))

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

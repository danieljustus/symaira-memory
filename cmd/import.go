package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/danieljustus/symaira-memory/internal/importer"
	"github.com/danieljustus/symaira-memory/internal/importer/aider"
	"github.com/danieljustus/symaira-memory/internal/importer/calendar"
	"github.com/danieljustus/symaira-memory/internal/importer/claudecode"
	"github.com/danieljustus/symaira-memory/internal/importer/codex"
	"github.com/danieljustus/symaira-memory/internal/importer/curatedmemory"
	"github.com/danieljustus/symaira-memory/internal/importer/email"
	"github.com/danieljustus/symaira-memory/internal/importer/git"
	"github.com/danieljustus/symaira-memory/internal/importer/github"
	"github.com/danieljustus/symaira-memory/internal/importer/hermes"
	"github.com/danieljustus/symaira-memory/internal/importer/memorytool"
	"github.com/danieljustus/symaira-memory/internal/importer/obsidian"
	"github.com/danieljustus/symaira-memory/internal/importer/opencode"
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

Supported tools: claude-code, codex, hermes, aider, curated-memory, git, github,
shell-history, calendar, email, obsidian, paperless, opencode, openmemory, mem0,
chatgpt.

Examples:
  symmemory import --tool claude-code
  symmemory import --tool curated-memory
  symmemory import --all
  symmemory import --tool claude-code --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if importList {
			cfg := GetConfig()
			tools := []string{"claude-code", "codex", "hermes", "aider", "curated-memory", "git", "github", "shell-history", "calendar", "email", "obsidian", "paperless", "opencode", "openmemory", "mem0", "chatgpt"}

			type toolStatus struct {
				Tool   string `json:"tool"`
				Status string `json:"status"`
				Path   string `json:"path,omitempty"`
			}
			var statuses []toolStatus
			for _, tool := range tools {
				tc, ok := cfg.Import.Tools[tool]
				status := "not configured"
				path := ""
				if ok && tc.Path != "" {
					status = "configured"
					path = tc.Path
				} else if ok {
					status = "configured (no path)"
				}
				if GetOutputFormat(cmd) == "json" {
					statuses = append(statuses, toolStatus{Tool: tool, Status: status, Path: path})
				} else {
					fmt.Printf("%-15s %s\n", tool, status)
				}
			}
			if GetOutputFormat(cmd) == "json" {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(statuses); err != nil {
					return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to encode JSON output")
				}
			}
			return nil
		}

		if !importAll && importTool == "" {
			return exitcodes.Wrapf(nil, exitcodes.ExitNoInput, exitcodes.KindValidation, "either --tool, --all, or --list flag is required")
		}

		registry := importer.NewRegistry(GetDB(), extractor.NewEmbeddingsGenerator(GetConfig()), GetConfig().Import.ExtractOnImport)

		cfg := GetConfig()

		// Session importers
		registry.Register(claudecode.NewClaudeCodeImporter(""))
		registry.Register(codex.NewCodexImporter(""))
		registry.Register(hermes.NewHermesImporter(""))
		registry.Register(opencode.NewOpenCodeImporter(""))
		registry.Register(aider.NewAiderImporter(nil))
		registry.Register(curatedmemory.NewCuratedMemoryImporter(""))

		// Data source importers
		registry.Register(git.NewGitImporter("", ""))
		registry.Register(github.NewGitHubImporter("", "", ""))
		registry.Register(shellhistory.NewShellHistoryImporter("", false, nil))
		registry.Register(calendar.NewCalendarImporter("", "", false, 7))
		registry.Register(email.NewEmailImporter("", "", 0))
		registry.Register(obsidian.NewObsidianImporter("", "", nil, nil, nil))
		registry.Register(paperless.NewPaperlessImporter("", "", "", "", 0))

		registry.Register(memorytool.NewSessionAdapter(memorytool.NewOpenMemoryImporter(), cfg.Import.Tools["openmemory"].Path))
		registry.Register(memorytool.NewSessionAdapter(memorytool.NewMem0Importer(), cfg.Import.Tools["mem0"].Path))
		registry.Register(memorytool.NewSessionAdapter(memorytool.NewChatGPTImporter(), cfg.Import.Tools["chatgpt"].Path))

		var tools []string
		if importAll {
			tools = registry.List()
		} else {
			tools = []string{importTool}
		}

		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		results, err := registry.RunImport(ctx, tools, importDryRun)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "import failed")
		}

		if GetOutputFormat(cmd) == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(results); err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to encode JSON output")
			}
			return nil
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
		return nil
	},
}

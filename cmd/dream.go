package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-memory/internal/consolidation"
	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/spf13/cobra"
)

var (
	dreamDryRun bool
	dreamScope  string
	dreamFormat string
)

func init() {
	dreamCmd.Flags().BoolVar(&dreamDryRun, "dry-run", false, "Show what would be consolidated without applying changes")
	dreamCmd.Flags().StringVar(&dreamScope, "scope", "", "Filter by scope (e.g., global, agent, project)")
	dreamCmd.Flags().StringVar(&dreamFormat, "format", "text", "Output format: text or json")
	rootCmd.AddCommand(dreamCmd)
}

type dreamOutput struct {
	Scope       string `json:"scope"`
	NewMemories int    `json:"new_memories"`
	Archived    int    `json:"archived"`
	DryRun      bool   `json:"dry_run"`
}

var dreamCmd = &cobra.Command{
	Use:   "dream",
	Short: "Run memory consolidation (dreaming)",
	Long: `Run the consolidation engine to merge, deduplicate, and synthesize raw
memory facts into consolidated knowledge. This process groups related memories,
resolves contradictions, and creates higher-quality synthesized facts.

Examples:
  symmemory dream
  symmemory dream --dry-run
  symmemory dream --scope agent
  symmemory dream --format json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := GetConfig()
		db := GetDB()

		llmURL := cfg.Consolidation.URL
		llmModel := cfg.Consolidation.Model
		llmProvider := cfg.Consolidation.Provider
		if llmProvider == "" {
			llmProvider = "ollama"
		}
		if llmURL == "" {
			llmURL = cfg.Ollama.URL
		}
		if llmModel == "" {
			llmModel = cfg.Ollama.Model
		}

		piiEnabled := false
		if cfg.Security.PIIEnabled != nil {
			piiEnabled = *cfg.Security.PIIEnabled
		}

		embeddings := extractor.NewEmbeddingsGenerator(cfg)

		engine := consolidation.NewEngine(db, embeddings, llmURL, llmModel, llmProvider, piiEnabled)

		summaries, err := engine.RunConsolidation(cmd.Context(), dreamScope, dreamDryRun)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "consolidation failed")
		}

		if len(summaries) == 0 {
			fmt.Fprintln(os.Stderr, "No raw memories to consolidate")
			return nil
		}

		if dreamFormat == "json" {
			output := make([]dreamOutput, 0, len(summaries))
			for _, s := range summaries {
				output = append(output, dreamOutput{
					Scope:       s.Scope,
					NewMemories: len(s.NewMemories),
					Archived:    len(s.ArchivedMemoryIDs),
					DryRun:      dreamDryRun,
				})
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(output); err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "JSON encode failed")
			}
		} else {
			for _, s := range summaries {
				fmt.Fprintf(os.Stdout, "Scope: %s\n", s.Scope)
				fmt.Fprintf(os.Stdout, "  New memories: %d\n", len(s.NewMemories))
				fmt.Fprintf(os.Stdout, "  Archived: %d\n", len(s.ArchivedMemoryIDs))
				if dreamDryRun {
					fmt.Fprintln(os.Stdout, "  (dry run - no changes made)")
				}
				fmt.Fprintln(os.Stdout)
			}
		}
		return nil
	},
}

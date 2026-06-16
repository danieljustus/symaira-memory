package cmd

import (
	"fmt"
	"os"

	"github.com/danieljustus/symaira-memory/internal/consolidation"
	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/spf13/cobra"
)

var (
	dreamDryRun bool
	dreamScope  string
)

func init() {
	dreamCmd.Flags().BoolVar(&dreamDryRun, "dry-run", false, "Show what would be consolidated without applying changes")
	dreamCmd.Flags().StringVar(&dreamScope, "scope", "", "Filter by scope (e.g., global, agent, project)")
	rootCmd.AddCommand(dreamCmd)
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
  symmemory dream --scope agent`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg := GetConfig()
		db := GetDB()

		llmURL := cfg.Ollama.URL
		llmModel := cfg.Ollama.Model
		llmProvider := cfg.Consolidation.Provider
		if llmProvider == "" {
			llmProvider = "ollama"
		}
		if cfg.Consolidation.Model != "" {
			llmModel = cfg.Consolidation.Model
		}

		piiEnabled := false
		if cfg.Security.PIIEnabled != nil {
			piiEnabled = *cfg.Security.PIIEnabled
		}

		embeddings := extractor.NewEmbeddingsGenerator(cfg)

		engine := consolidation.NewEngine(db, embeddings, llmURL, llmModel, llmProvider, piiEnabled)

		summaries, err := engine.RunConsolidation(cmd.Context(), dreamScope, dreamDryRun)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Consolidation failed: %v\n", err)
			os.Exit(1)
		}

		if len(summaries) == 0 {
			fmt.Fprintln(os.Stderr, "No raw memories to consolidate")
			return
		}

		for _, s := range summaries {
			fmt.Fprintf(os.Stderr, "Scope: %s\n", s.Scope)
			fmt.Fprintf(os.Stderr, "  New memories: %d\n", len(s.NewMemories))
			fmt.Fprintf(os.Stderr, "  Archived: %d\n", len(s.ArchivedMemoryIDs))
			if dreamDryRun {
				fmt.Fprintln(os.Stderr, "  (dry run - no changes made)")
			}
			fmt.Fprintln(os.Stderr)
		}
	},
}

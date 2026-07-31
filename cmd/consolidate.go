package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-memory/internal/consolidation"
	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/spf13/cobra"
)

var (
	consolidateDryRun bool
	consolidateUndo   bool
	consolidateScope  string
)

func init() {
	consolidateCmd.Flags().BoolVar(&consolidateDryRun, "dry-run", false, "Show what would be consolidated without making changes")
	consolidateCmd.Flags().BoolVar(&consolidateUndo, "undo", false, "Undo the last completed consolidation run")
	consolidateCmd.Flags().StringVar(&consolidateScope, "scope", "", "Only consolidate memories in this scope")
	rootCmd.AddCommand(consolidateCmd)
}

var consolidateCmd = &cobra.Command{
	Use:   "consolidate",
	Short: "Run memory consolidation or undo the last run",
	Long: `Runs memory consolidation: finds raw memories, groups them by scope,
prompts the LLM to merge duplicates and discard transient facts, and archives
the originals. Use --undo to reverse the last completed run.

Use --dry-run to preview what would happen without making changes.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := GetConfig()
		database := GetDB()
		embeddings := extractor.NewEmbeddingsGenerator(cfg)
		piiEnabled := cfg.Security.PIIEnabled != nil && *cfg.Security.PIIEnabled
		engine := consolidation.NewEngine(database, embeddings, cfg.Consolidation.URL, cfg.Consolidation.Model, cfg.Consolidation.Provider, piiEnabled)

		if consolidateUndo {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			summary, err := engine.UndoLastConsolidation(ctx)
			if err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "undo consolidation failed")
			}
			fmt.Println(summary)
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		summaries, err := engine.RunConsolidation(ctx, consolidateScope, consolidateDryRun)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "consolidation failed")
		}

		if len(summaries) == 0 {
			fmt.Println("No raw memories to consolidate.")
			return nil
		}

		outputFmt := GetOutputFormat(cmd)
		if outputFmt == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(summaries)
		}

		totalNew := 0
		totalArchived := 0
		for _, s := range summaries {
			totalNew += len(s.NewMemories)
			totalArchived += len(s.ArchivedMemoryIDs)
			fmt.Printf("Scope %q:\n", s.Scope)
			fmt.Printf("  New consolidated memories: %d\n", len(s.NewMemories))
			fmt.Printf("  Archived originals: %d\n", len(s.ArchivedMemoryIDs))
		}
		fmt.Printf("\nTotal: %d new consolidated memory(ies), %d archived original(s)\n", totalNew, totalArchived)
		if consolidateDryRun {
			fmt.Println("(dry run — no changes made)")
		}
		return nil
	},
}

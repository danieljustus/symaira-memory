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
	"github.com/danieljustus/symaira-memory/internal/working"
	"github.com/spf13/cobra"
)

var (
	compactDryRun bool
	compactOutput string
)

func init() {
	compactCmd.Flags().BoolVar(&compactDryRun, "dry-run", false, "Show what would be compacted without making changes")
	compactCmd.Flags().StringVar(&compactOutput, "output", "table", "Output format: table or json")
	rootCmd.AddCommand(compactCmd)
}

var compactCmd = &cobra.Command{
	Use:   "compact",
	Short: "Compact expired working memories via consolidation and eviction",
	Long: `Compacts expired working memories by feeding them through the consolidation
engine and then evicting the originals. Use --dry-run to preview without changes.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := GetConfig()
		database := GetDB()
		embeddings := extractor.NewEmbeddingsGenerator(cfg)
		engine := consolidation.NewEngine(database, embeddings, cfg.Consolidation.URL, cfg.Consolidation.Model, cfg.Consolidation.Provider, cfg.Security.PIIEnabled != nil && *cfg.Security.PIIEnabled)
		evictor := working.NewEvictor(database, embeddings, engine, cfg.Security.PIIEnabled != nil && *cfg.Security.PIIEnabled)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		result, err := evictor.CompactWorkingMemories(ctx, compactDryRun)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "compaction failed")
		}

		if compactOutput == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}

		fmt.Printf("Compaction result:\n")
		fmt.Printf("  Expired: %d\n", result.ExpiredCount)
		fmt.Printf("  Evicted: %d\n", result.EvictedCount)
		if compactDryRun {
			fmt.Println("  (dry run — no changes made)")
		}
		return nil
	},
}

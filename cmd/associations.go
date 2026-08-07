package cmd

import (
	"fmt"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/spf13/cobra"
)

var (
	associationsDryRun bool
)

func init() {
	associationsSeedCmd.Flags().BoolVar(&associationsDryRun, "dry-run", false, "Report how many edges would be created without writing")
	associationsCmd.AddCommand(associationsSeedCmd)
	rootCmd.AddCommand(associationsCmd)
}

var associationsCmd = &cobra.Command{
	Use:   "associations",
	Short: "Manage memory-to-memory associations",
	Long: `Manages the weighted memory-to-memory association graph (#488): edges
between memories that let a strong retrieval hit lift directly related
facts that would otherwise never surface. Edges are seeded from signals
the store already records (co-retrieval in the query log, shared entity
links, consolidation siblings) and are consumed by the retrieval
spreading term when ranking.spreading_weight is enabled.`,
}

var associationsSeedCmd = &cobra.Command{
	Use:   "seed",
	Short: "Derive association edges from query log, entities, and consolidation",
	RunE: func(cmd *cobra.Command, args []string) error {
		database := GetDB()
		if database == nil {
			return exitcodes.Wrapf(fmt.Errorf("database not available"), exitcodes.ExitSoftware, exitcodes.KindInternal, "associations seed failed")
		}
		// Dry-run: report the current edge count and the seed sources;
		// the derivation itself is idempotent (upserts keep max weight),
		// so a real run is always safe to re-run.
		if associationsDryRun {
			n, err := database.AssociationCount()
			if err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "associations seed failed")
			}
			fmt.Printf("associations: %d edge(s) stored; seeding is idempotent and derives edges from co-retrieval, shared entities, and consolidation siblings [dry-run, no writes]\n", n)
			return nil
		}
		before, err := database.AssociationCount()
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "associations seed failed")
		}
		inserted, err := database.SeedMemoryAssociations("cli")
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "associations seed failed")
		}
		after, err := database.AssociationCount()
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "associations seed failed")
		}
		fmt.Printf("associations seeded: %d upsert(s) attempted, edge count %d -> %d\n", inserted, before, after)
		return nil
	},
}

package cmd

import (
	"fmt"
	"time"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-memory/internal/aging"
	"github.com/spf13/cobra"
)

var (
	agingDryRun bool
)

func init() {
	agingRunCmd.Flags().BoolVar(&agingDryRun, "dry-run", false, "Show what the aging pass would do without writing anything")
	agingCmd.AddCommand(agingRunCmd)
	rootCmd.AddCommand(agingCmd)
}

var agingCmd = &cobra.Command{
	Use:   "aging",
	Short: "Run the memory aging pass (decay + retirement)",
	Long: `Runs the explicit memory aging pass: per-fact decay derived from last
access, access count and creation time is written back and multiplied into
the retrieval score, and memories whose decay drops below the configured
floor are retired by flagging them (never hard-deleted).

Use 'aging run --dry-run' to preview the pass without any writes. The pass
runs alongside consolidation, not inside it, and is fully reversible:
retirement is a flag and decay can be pinned to 1.0 via config
(aging.enabled = false).`,
}

var agingRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Execute the aging pass over all non-retired memories",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := GetConfig()
		database := GetDB()
		if database == nil {
			return exitcodes.Wrapf(fmt.Errorf("database not available"), exitcodes.ExitSoftware, exitcodes.KindInternal, "aging run failed")
		}
		report, err := aging.Run(database, aging.FromConfig(cfg.Aging), time.Now(), agingDryRun)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "aging run failed")
		}
		fmt.Println(report.String())
		return nil
	},
}

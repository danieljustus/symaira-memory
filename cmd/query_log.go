package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/spf13/cobra"
)

var (
	queryLogLimit int
	queryLogJSON  bool
)

func init() {
	queryLogCmd.Flags().IntVarP(&queryLogLimit, "limit", "n", 20, "Number of recent entries to show")
	queryLogCmd.Flags().BoolVar(&queryLogJSON, "json", false, "Output raw JSON instead of table")
	rootCmd.AddCommand(queryLogCmd)
}

var queryLogCmd = &cobra.Command{
	Use:   "query-log",
	Short: "Show a summary of recent MCP tool calls (query log)",
	Long: `Display the query log summary — tool call breakdown and recent entries.
The log is bounded at 1000 entries; oldest entries are pruned automatically.`,
	Example: `  # Show query log summary
  symmemory query-log

  # Show more recent entries
  symmemory query-log --limit 50

  # Output as JSON
  symmemory query-log --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		db := GetDB()
		if db == nil {
			return exitcodes.Wrapf(nil, exitcodes.ExitSoftware, exitcodes.KindInternal, "database not initialized")
		}

		summary, err := db.GetQueryLogSummary(queryLogLimit)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to get query log summary")
		}

		if queryLogJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(summary)
			return nil
		}

		// Table output
		fmt.Printf("Query Log Summary\n")
		fmt.Printf("Total queries: %d\n\n", summary.TotalQueries)

		if len(summary.ToolBreakdown) > 0 {
			fmt.Println("Tool Breakdown:")
			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "  Tool\tCount")
			fmt.Fprintln(tw, "  ----\t-----")
			for _, tool := range sortedKeys(summary.ToolBreakdown) {
				fmt.Fprintf(tw, "  %s\t%d\n", tool, summary.ToolBreakdown[tool])
			}
			_ = tw.Flush()
			fmt.Println()
		}

		if len(summary.RecentEntries) > 0 {
			fmt.Printf("Recent Entries (last %d):\n", len(summary.RecentEntries))
			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "  Tool\tQuery\tDuration\tTimestamp")
			fmt.Fprintln(tw, "  ----\t-----\t--------\t---------")
			for _, e := range summary.RecentEntries {
				query := e.QueryText
				if len(query) > 60 {
					query = query[:60] + "..."
				}
				ts := e.CreatedAt.Format("15:04:05")
				fmt.Fprintf(tw, "  %s\t%s\t%vms\t%s\n", e.Tool, query, e.DurationMs, ts)
			}
			_ = tw.Flush()
		}

		return nil
	},
}

// sortedKeys returns the keys of m in insertion order (map iteration is
// already stable in Go 1.21+ for the same map, but we sort for readability).
func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Simple sort (just alphabetical for tool names)
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

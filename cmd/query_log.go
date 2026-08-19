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
	queryLogActor string
)

func init() {
	queryLogCmd.Flags().IntVarP(&queryLogLimit, "limit", "n", 20, "Number of recent entries to show")
	queryLogCmd.Flags().BoolVar(&queryLogJSON, "json", false, "Deprecated: use --output json")
	_ = queryLogCmd.Flags().MarkHidden("json")
	_ = queryLogCmd.Flags().MarkDeprecated("json", "use --output json")
	queryLogCmd.Flags().StringVar(&queryLogActor, "actor", "", "Only show entries recorded for this actor (client identity)")
	queryLogCmd.AddCommand(queryLogResultsCmd)
	rootCmd.AddCommand(queryLogCmd)
}

// queryLogResultsCmd resolves a logged query to the memories it returned.
// This is the read side of query_log_results (issue #460): every recorded
// row is a reference (memory id + rank + score), never a content copy.
var queryLogResultsCmd = &cobra.Command{
	Use:   "results [query-id]",
	Short: "Show the memories a logged query returned",
	Long: `Show which memories a logged query returned, one row per memory with
its retrieval rank and score. The query id is the id column of
'symmemory query-log' (JSON output shows it per entry). Recording is
governed by [query_log] record_results (default true).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database := GetDB()
		if database == nil {
			return exitcodes.Wrapf(nil, exitcodes.ExitSoftware, exitcodes.KindInternal, "database not initialized")
		}

		results, err := database.GetQueryLogResults(args[0])
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to read query log results")
		}

		if queryLogJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(results)
			return nil
		}

		if len(results) == 0 {
			fmt.Printf("No recorded results for query %s (recording off, query pruned, or the search returned nothing).\n", args[0])
			return nil
		}
		fmt.Printf("Query %s returned %d memory reference(s):\n", args[0], len(results))
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "  Rank	Score	Memory ID")
		fmt.Fprintln(tw, "  ----	-----	---------")
		for _, r := range results {
			fmt.Fprintf(tw, "  %d	%.4f	%s\n", r.Rank, r.Score, r.MemoryID)
		}
		_ = tw.Flush()
		return nil
	},
}

var queryLogCmd = &cobra.Command{
	Use:   "query-log",
	Short: "Show a summary of recent MCP tool calls (query log)",
	Long: `Display the query log summary — tool and actor breakdowns and recent entries.
Each entry records which client asked (actor), in which scope and session.
The log is bounded; the cap (default 1000 entries) and an optional max age
are configured under the [query_log] section. Oldest entries are pruned
automatically on write.`,
	Example: `  # Show query log summary
  symmemory query-log

  # Show more recent entries
  symmemory query-log --limit 50

  # Only show entries recorded for a specific client
  symmemory query-log --actor "claude/1.0"

  # Output as JSON
  symmemory query-log --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		db := GetDB()
		if db == nil {
			return exitcodes.Wrapf(nil, exitcodes.ExitSoftware, exitcodes.KindInternal, "database not initialized")
		}

		summary, err := db.GetQueryLogSummary(queryLogLimit, queryLogActor)
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
			fmt.Fprintln(tw, "  Tool	Count")
			fmt.Fprintln(tw, "  ----	-----")
			for _, tool := range sortedKeys(summary.ToolBreakdown) {
				fmt.Fprintf(tw, "  %s	%d\n", tool, summary.ToolBreakdown[tool])
			}
			_ = tw.Flush()
			fmt.Println()
		}

		if len(summary.ActorBreakdown) > 0 {
			fmt.Println("Actor Breakdown:")
			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "  Actor	Count")
			fmt.Fprintln(tw, "  -----	-----")
			for _, actor := range sortedKeys(summary.ActorBreakdown) {
				fmt.Fprintf(tw, "  %s	%d\n", actor, summary.ActorBreakdown[actor])
			}
			_ = tw.Flush()
			fmt.Println()
		}

		if len(summary.RecentEntries) > 0 {
			fmt.Printf("Recent Entries (last %d):\n", len(summary.RecentEntries))
			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "  Actor	Tool	Query	Duration	Timestamp")
			fmt.Fprintln(tw, "  -----	----	-----	--------	---------")
			for _, e := range summary.RecentEntries {
				query := e.QueryText
				if len(query) > 60 {
					query = query[:60] + "..."
				}
				ts := e.CreatedAt.Format("15:04:05")
				fmt.Fprintf(tw, "  %s	%s	%s	%vms	%s\n", e.Actor, e.Tool, query, e.DurationMs, ts)
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

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/danieljustus/symaira-memory/internal/bench"
	"github.com/danieljustus/symaira-memory/internal/summarizer"
	"github.com/spf13/cobra"
)

var (
	benchRepetitions      int
	benchFixture          string
	benchDataset          string
	benchCorpus           string
	benchAbstainThreshold float64
	benchSnapshot         bool
	benchCompare          bool
	benchRepeatRuns       int
	benchSeed             int64
)

func init() {
	benchCmd.Flags().IntVarP(&benchRepetitions, "repetitions", "n", 10, "Number of repetitions for latency measurement")
	benchCmd.Flags().StringVar(&benchFixture, "fixture", "", "Path to custom JSON/YAML fixture file (optional)")
	benchCmd.Flags().StringVar(&benchFixture, "path", "", "Path to a locally downloaded dataset file (e.g. LongMemEval JSON)")
	benchCmd.Flags().StringVar(&benchDataset, "dataset", "", "External dataset name for opt-in evaluation (optional; alias for --corpus)")
	benchCmd.Flags().StringVar(&benchCorpus, "corpus", "", "Corpus to evaluate: builtin (default) or longmemeval")
	benchCmd.Flags().Float64Var(&benchAbstainThreshold, "abstain-threshold", 0, "Score threshold for abstention evaluation on corpora with unanswerable queries")
	benchCmd.Flags().BoolVar(&benchSnapshot, "snapshot", false, "Save benchmark results as a baseline snapshot under .bench-snapshots/")
	benchCmd.Flags().BoolVar(&benchCompare, "compare", false, "Compare benchmark results against the stored baseline snapshot")
	benchCmd.Flags().IntVar(&benchRepeatRuns, "repeat-runs", 1, "Rerun the query evaluation this many times and pool per-query samples")
	benchCmd.Flags().Int64Var(&benchSeed, "seed", 42, "Seed for the deterministic bootstrap resampling in comparisons")
	rootCmd.AddCommand(benchCmd)
}

var benchCmd = &cobra.Command{
	Use:   "bench",
	Short: "Run corpus-backed retrieval benchmark measuring BM25, vector, and hybrid KPIs",
	Long: `Runs a reproducible retrieval benchmark against a built-in deterministic corpus:
  - BM25 keyword search quality and latency
  - Vector semantic search quality and latency
  - Hybrid (RRF-fused) search quality and latency
  - Temporal-validity evaluation (expired vs currently-valid memories)
  - Scope-isolation evaluation (query constrained to specific scopes)

Metrics reported: Recall@k, NDCG@k, MRR, latency percentiles (P50/P95).

The default run uses a built-in fixture corpus so CI is deterministic
without network access. Use --dataset for external evaluation sets.

Flags:
  --snapshot           Save results as a baseline snapshot in .bench-snapshots/
                       (versioned JSON with mode, metrics, timestamp, corpus hash)
  --compare            Compare results against the stored baseline snapshot
                       and report any regressions

All output goes to stderr for easy piping.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(os.Stderr, "--- Token Reduction (Summarizer) ---\n")
		benchTokenReduction()

		fmt.Fprintf(os.Stderr, "\n--- Retrieval Benchmark ---\n")
		benchOutput := GetOutputFormat(cmd)
		if benchOutput != "json" {
			benchOutput = "text"
		}

		// Resolve corpus name for snapshot/compare operations
		corpusName := benchCorpus
		if corpusName == "" {
			corpusName = benchDataset
		}
		if corpusName == "" {
			corpusName = "builtin"
		}

		opts := bench.Options{
			Repetitions:      benchRepetitions,
			Output:           "json", // always capture JSON internally for snapshot/compare
			FixturePath:      benchFixture,
			Dataset:          benchDataset,
			Corpus:           benchCorpus,
			AbstainThreshold: benchAbstainThreshold,
			RepeatRuns:       benchRepeatRuns,
			Seed:             benchSeed,
		}

		var buf bytes.Buffer
		if err := bench.Run(&buf, opts); err != nil {
			fmt.Fprintf(os.Stderr, "Benchmark failed: %v\n", err)
			os.Exit(1)
		}

		var report bench.Report
		if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to parse benchmark report: %v\n", err)
			os.Exit(1)
		}

		// Display the report in the requested format
		switch benchOutput {
		case "json":
			var pretty bytes.Buffer
			if err := json.Indent(&pretty, buf.Bytes(), "", "  "); err == nil {
				fmt.Fprintf(os.Stderr, "%s\n", pretty.String())
			} else {
				fmt.Fprintf(os.Stderr, "%s\n", buf.String())
			}
		default:
			if err := writeBenchTextReport(os.Stderr, report); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to write text report: %v\n", err)
			}
		}

		// Handle --snapshot and --compare
		if benchSnapshot {
			handleSnapshot(report, corpusName)
		}
		if benchCompare {
			handleCompare(report, corpusName)
		}
	},
}

// handleSnapshot saves the benchmark report as a baseline snapshot.
func handleSnapshot(report bench.Report, corpusName string) {
	// Compute corpus hash from the report's corpus — we need to reconstruct
	// it from the default corpus since the report doesn't carry the raw corpus.
	// For consistency, we compute it from the standard corpus matching the name.
	corpus := resolveCorpus(corpusName)
	corpusHash := bench.ComputeCorpusHash(corpus)

	snap := report.ToSnapshotFile(corpusName, corpusHash)
	path, err := bench.SaveSnapshot(bench.SnapshotsDir, snap)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save snapshot: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "\nBaseline snapshot saved: %s\n", path)
	fmt.Fprintf(os.Stderr, "Corpus hash: %s\n", corpusHash[:12])
}

// handleCompare runs the comparison and writes the report.
func handleCompare(report bench.Report, corpusName string) {
	corpus := resolveCorpus(corpusName)
	corpusHash := bench.ComputeCorpusHash(corpus)

	baseline, _, err := bench.FindBaselineSnapshot(bench.SnapshotsDir, corpusName, corpusHash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading baseline snapshot: %v\n", err)
		os.Exit(1)
	}
	if baseline.SchemaVersion == 0 {
		fmt.Fprintf(os.Stderr, "\nNo baseline snapshot found for corpus %q (hash: %s).\n", corpusName, corpusHash[:12])
		fmt.Fprintf(os.Stderr, "Run with --snapshot first to create a baseline.\n")
		os.Exit(1)
	}

	current := report.ToSnapshotFile(corpusName, corpusHash)
	comparison := bench.CompareSnapshots(baseline, current)

	fmt.Fprintf(os.Stderr, "\n")
	bench.WriteComparisonReport(os.Stderr, comparison)

	if !comparison.Passed {
		os.Exit(1)
	}
}

// resolveCorpus returns a corpus matching the given name for hash computation.
func resolveCorpus(name string) *bench.Corpus {
	switch name {
	case "builtin":
		return bench.DefaultCorpus()
	default:
		// For unknown corpora, return the default for hash computation.
		// This is conservative — external corpora can't be hashed without loading.
		return bench.DefaultCorpus()
	}
}

func benchTokenReduction() {
	sessionText := `User: What port does the backend use?
Assistant: The backend uses port 8080 for the HTTP API.
User: And what about the database?
Assistant: The database is SQLite stored at ~/.local/share/symmemory/default.db
User: Great, and the embedding model?
Assistant: We use nomic-embed-text via Ollama with a fallback to FNV-1a hash vectors.
User: Thanks for the architecture overview!
Assistant: You're welcome! Let me know if you need more details about any component.`

	originalTokens := estimateBenchTokens(sessionText)

	sum := summarizer.NewExtractiveSummarizer()
	summary := sum.SummarizeSession(sessionText, 3)
	summaryTokens := estimateBenchTokens(summary)

	reduction := 0.0
	if originalTokens > 0 {
		reduction = float64(originalTokens-summaryTokens) / float64(originalTokens) * 100
	}

	fmt.Fprintf(os.Stderr, "  Original tokens:    %d\n", originalTokens)
	fmt.Fprintf(os.Stderr, "  Summary tokens:     %d\n", summaryTokens)
	fmt.Fprintf(os.Stderr, "  Reduction:          %.1f%%\n", reduction)
	fmt.Fprintf(os.Stderr, "  Summary output:     %s\n", truncateStr(summary, 100))
}

func estimateBenchTokens(text string) int {
	if text == "" {
		return 0
	}
	words := make([]string, 0)
	for _, w := range splitWords(text) {
		if w != "" {
			words = append(words, w)
		}
	}
	return (len(words) * 4) / 3
}

func splitWords(text string) []string {
	var words []string
	var current []rune
	for _, r := range text {
		if r == ' ' || r == '\n' || r == '\t' || r == '\r' {
			if len(current) > 0 {
				words = append(words, string(current))
				current = nil
			}
		} else {
			current = append(current, r)
		}
	}
	if len(current) > 0 {
		words = append(words, string(current))
	}
	return words
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// writeBenchTextReport writes a human-readable benchmark report to w.
func writeBenchTextReport(w *os.File, report bench.Report) error {
	buf := &bytes.Buffer{}
	writeLine := func(format string, args ...interface{}) {
		fmt.Fprintf(buf, format, args...)
	}

	writeLine("=== Symaira Memory Retrieval Benchmark ===\n\n")
	writeLine("Corpus:     %d memories\n", report.CorpusSize)
	writeLine("Queries:    %d evaluation queries\n", report.QueryCount)
	writeLine("Reps:       %d per query for latency\n", report.Repetitions)
	writeLine("Embeddings: %s\n\n", report.EmbeddingSource)

	writeLine("                    Recall@5  Recall@10  NDCG@5   NDCG@10  MRR      P50(ms)  P95(ms)\n")
	writeLine("─────────────────  ────────  ────────   ───────  ───────  ───────  ───────  ───────\n")

	for _, m := range []bench.RetrievalMetrics{report.BM25, report.Vector, report.Hybrid} {
		writeLine("  %-16s %7.3f   %7.3f    %6.3f   %6.3f   %6.3f   %7.2f  %7.2f\n",
			m.Mode, m.RecallAt5, m.RecallAt10, m.NDCGAt5, m.NDCGAt10, m.MRR,
			m.P50LatencyMs, m.P95LatencyMs)
	}

	writeLine("\n--- Temporal Validity ---\n")
	for _, t := range report.Temporal {
		writeLine("  %s\n", t.Description)
	}

	writeLine("\n--- Scope Isolation ---\n")
	for _, s := range report.Scope {
		writeLine("  %s\n", s.Description)
	}

	if len(report.Abstention) > 0 {
		writeLine("\n--- Abstention ---\n")
		for _, a := range report.Abstention {
			writeLine("  [mode=%s] threshold=%.3f: %d/%d correct (accuracy %.3f)\n",
				a.Mode, a.Threshold, a.Correct, a.Total, a.Accuracy)
		}
	}

	writeLine("\n=== Benchmark Complete ===\n")

	_, err := fmt.Fprint(w, buf.String())
	return err
}

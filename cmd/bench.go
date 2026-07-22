package cmd

import (
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
)

func init() {
	benchCmd.Flags().IntVarP(&benchRepetitions, "repetitions", "n", 10, "Number of repetitions for latency measurement")
	benchCmd.Flags().StringVar(&benchFixture, "fixture", "", "Path to custom JSON/YAML fixture file (optional)")
	benchCmd.Flags().StringVar(&benchFixture, "path", "", "Path to a locally downloaded dataset file (e.g. LongMemEval JSON)")
	benchCmd.Flags().StringVar(&benchDataset, "dataset", "", "External dataset name for opt-in evaluation (optional; alias for --corpus)")
	benchCmd.Flags().StringVar(&benchCorpus, "corpus", "", "Corpus to evaluate: builtin (default) or longmemeval")
	benchCmd.Flags().Float64Var(&benchAbstainThreshold, "abstain-threshold", 0, "Score threshold for abstention evaluation on corpora with unanswerable queries")
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
		opts := bench.Options{
			Repetitions:      benchRepetitions,
			Output:           benchOutput,
			FixturePath:      benchFixture,
			Dataset:          benchDataset,
			Corpus:           benchCorpus,
			AbstainThreshold: benchAbstainThreshold,
		}
		if err := bench.Run(os.Stderr, opts); err != nil {
			fmt.Fprintf(os.Stderr, "Benchmark failed: %v\n", err)
			os.Exit(1)
		}
	},
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

package cmd

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danieljustus/symaira-memory/internal/bench"
)

// testBenchReport returns a deterministic benchmark report for handler tests.
func testBenchReport() bench.Report {
	return bench.Report{
		Timestamp:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		CorpusSize:      50,
		QueryCount:      12,
		Repetitions:     5,
		EmbeddingSource: "hash-fallback",
		BM25: bench.RetrievalMetrics{
			Mode: "bm25", RecallAt5: 0.85, RecallAt10: 0.90,
			NDCGAt5: 0.82, NDCGAt10: 0.88, MRR: 0.92,
			P50LatencyMs: 0.5, P95LatencyMs: 1.0,
		},
		Vector: bench.RetrievalMetrics{
			Mode: "vector", RecallAt5: 0.75, RecallAt10: 0.82,
			NDCGAt5: 0.72, NDCGAt10: 0.79, MRR: 0.80,
			P50LatencyMs: 2.0, P95LatencyMs: 4.5,
		},
		Hybrid: bench.RetrievalMetrics{
			Mode: "hybrid", RecallAt5: 0.90, RecallAt10: 0.95,
			NDCGAt5: 0.88, NDCGAt10: 0.92, MRR: 0.95,
			P50LatencyMs: 1.2, P95LatencyMs: 3.0,
		},
	}
}

func TestHandleSnapshot(t *testing.T) {
	t.Chdir(t.TempDir())

	report := testBenchReport()
	out := captureStderr(func() {
		handleSnapshot(report, "builtin")
	})

	if !strings.Contains(out, "Baseline snapshot saved") {
		t.Errorf("expected 'Baseline snapshot saved' on stderr, got %q", out)
	}

	// The snapshot must be persisted under .bench-snapshots/ with the hash
	// derived from the default corpus.
	hash := bench.ComputeCorpusHash(bench.DefaultCorpus())
	path := bench.SnapshotFilePath(bench.SnapshotsDir, "builtin", hash)
	loaded, err := bench.LoadSnapshot(path)
	if err != nil {
		t.Fatalf("snapshot not written to %s: %v", path, err)
	}
	if loaded.CorpusName != "builtin" {
		t.Errorf("expected corpus 'builtin', got %q", loaded.CorpusName)
	}
	if len(loaded.Snapshots) != 3 {
		t.Errorf("expected 3 mode snapshots, got %d", len(loaded.Snapshots))
	}
	if loaded.Snapshots[0].Mode != "bm25" {
		t.Errorf("expected first mode 'bm25', got %q", loaded.Snapshots[0].Mode)
	}
}

func TestHandleCompareWithBaseline(t *testing.T) {
	t.Chdir(t.TempDir())

	report := testBenchReport()
	handleSnapshot(report, "builtin")

	out := captureStderr(func() {
		handleCompare(report, "builtin")
	})

	if !strings.Contains(out, "=== Benchmark Comparison ===") {
		t.Errorf("expected comparison report on stderr, got %q", out)
	}
	if !strings.Contains(out, "ALL METRICS PASSED") {
		t.Errorf("expected identical snapshots to pass comparison, got %q", out)
	}
}

// TestHandleCompareNoBaseline verifies that handleCompare exits with code 1
// when no baseline snapshot exists yet. The exit is exercised in a child
// process because os.Exit cannot be intercepted in-process.
func TestHandleCompareNoBaseline(t *testing.T) {
	if os.Getenv("GO_WANT_HANDLE_COMPARE_HELPER") == "1" {
		t.Chdir(t.TempDir())
		handleCompare(testBenchReport(), "builtin")
		t.Fatal("handleCompare should have exited with code 1")
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestHandleCompareNoBaseline$")
	cmd.Env = append(os.Environ(), "GO_WANT_HANDLE_COMPARE_HELPER=1")
	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected subprocess to exit non-zero, got %v", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.ExitCode())
	}
}

func TestResolveCorpus(t *testing.T) {
	builtin := resolveCorpus("builtin")
	if builtin == nil {
		t.Fatal("resolveCorpus('builtin') returned nil")
	}
	if len(builtin.Memories) != 50 {
		t.Errorf("expected 50 builtin memories, got %d", len(builtin.Memories))
	}

	// Unknown corpus names must fall back to the default corpus so hash
	// computation stays deterministic.
	fallback := resolveCorpus("nonexistent-corpus")
	if fallback == nil {
		t.Fatal("resolveCorpus('nonexistent-corpus') returned nil")
	}
	if h1, h2 := bench.ComputeCorpusHash(builtin), bench.ComputeCorpusHash(fallback); h1 != h2 {
		t.Error("expected unknown corpora to fall back to the default corpus")
	}
}

func TestWriteBenchTextReport(t *testing.T) {
	report := testBenchReport()
	report.Temporal = []bench.TemporalReport{
		{Mode: "hybrid", ValidFraction: 1.0, Description: "temporal validity ok"},
	}
	report.Scope = []bench.ScopeReport{
		{Mode: "hybrid", AllInScope: true, Description: "scope isolation ok"},
	}
	report.Abstention = []bench.AbstentionReport{
		{Mode: "hybrid", Threshold: 0.3, Correct: 9, Total: 10, Accuracy: 0.9},
	}

	path := filepath.Join(t.TempDir(), "report.txt")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create report file: %v", err)
	}
	if err := writeBenchTextReport(f, report); err != nil {
		t.Fatalf("writeBenchTextReport: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close report file: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report file: %v", err)
	}
	out := string(data)
	for _, want := range []string{
		"=== Symaira Memory Retrieval Benchmark ===",
		"50 memories",
		"12 evaluation queries",
		"bm25",
		"vector",
		"hybrid",
		"--- Temporal Validity ---",
		"temporal validity ok",
		"--- Scope Isolation ---",
		"scope isolation ok",
		"--- Abstention ---",
		"=== Benchmark Complete ===",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in text report, got:\n%s", want, out)
		}
	}
}

package bench

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SnapshotsDir is the directory where benchmark baseline snapshots are stored.
// Relative to the working directory when the benchmark is executed.
const SnapshotsDir = ".bench-snapshots"

// BenchSnapshot holds benchmark metrics for a single retrieval mode in a snapshot.
type BenchSnapshot struct {
	SchemaVersion int       `json:"schema_version"`
	Mode          string    `json:"mode"` // bm25, vector, hybrid
	RecallAt5     float64   `json:"recall_at_5"`
	RecallAt10    float64   `json:"recall_at_10"`
	NDCGAt5       float64   `json:"ndcg_at_5"`
	NDCGAt10      float64   `json:"ndcg_at_10"`
	MRR           float64   `json:"mrr"`
	P50LatencyMs  float64   `json:"p50_latency_ms"`
	P95LatencyMs  float64   `json:"p95_latency_ms"`
	Timestamp     time.Time `json:"timestamp"`
	CorpusHash    string    `json:"corpus_hash"`
	CorpusName    string    `json:"corpus_name,omitempty"`
}

// SnapshotFile is the complete set of mode snapshots from one benchmark run.
type SnapshotFile struct {
	SchemaVersion int            `json:"schema_version"`
	CreatedAt     time.Time      `json:"created_at"`
	CorpusHash    string         `json:"corpus_hash"`
	CorpusName    string         `json:"corpus_name"`
	CorpusSize    int            `json:"corpus_size"`
	QueryCount    int            `json:"query_count"`
	Snapshots     []BenchSnapshot `json:"snapshots"`
}

// ComputeCorpusHash computes a deterministic SHA-256 hash of a corpus.
// This allows detecting when the corpus changes between benchmark runs.
// The hash is based on memory IDs, scopes, content, and query relevance IDs.
func ComputeCorpusHash(corpus *Corpus) string {
	var b strings.Builder
	// Sort memories by ID for canonical ordering
	memIDs := make([]string, 0, len(corpus.Memories))
	memByID := make(map[string]FixtureMemory, len(corpus.Memories))
	for _, m := range corpus.Memories {
		memIDs = append(memIDs, m.ID)
		memByID[m.ID] = m
	}
	sort.Strings(memIDs)
	for _, id := range memIDs {
		m := memByID[id]
		fmt.Fprintf(&b, "%s|%s|%s\n", m.ID, m.Scope, m.Content)
	}

	// Sort queries by query text for canonical ordering
	qTexts := make([]string, 0, len(corpus.Queries))
	byQText := make(map[string]GroundTruth, len(corpus.Queries))
	for _, q := range corpus.Queries {
		qTexts = append(qTexts, q.Query)
		byQText[q.Query] = q
	}
	sort.Strings(qTexts)
	for _, qt := range qTexts {
		q := byQText[qt]
		sorted := make([]string, len(q.RelevantIDs))
		copy(sorted, q.RelevantIDs)
		sort.Strings(sorted)
		fmt.Fprintf(&b, "%s|%s\n", q.Query, strings.Join(sorted, ","))
	}

	hash := sha256.Sum256([]byte(b.String()))
	return fmt.Sprintf("%x", hash[:])
}

// SnapshotFileName returns the file name for a snapshot given corpus name and hash.
func SnapshotFileName(corpusName, corpusHash string) string {
	short := corpusHash
	if len(short) > 12 {
		short = short[:12]
	}
	return fmt.Sprintf("bench-snapshot-%s-%s.json", corpusName, short)
}

// SnapshotFilePath returns the full path to a snapshot file within snapDir.
func SnapshotFilePath(snapDir, corpusName, corpusHash string) string {
	return filepath.Join(snapDir, SnapshotFileName(corpusName, corpusHash))
}

// ToSnapshotFile converts a Report into the snapshot file format.
func (r *Report) ToSnapshotFile(corpusName, corpusHash string) SnapshotFile {
	sf := SnapshotFile{
		SchemaVersion: 1,
		CreatedAt:     r.Timestamp,
		CorpusHash:    corpusHash,
		CorpusName:    corpusName,
		CorpusSize:    r.CorpusSize,
		QueryCount:    r.QueryCount,
	}
	for _, m := range []RetrievalMetrics{r.BM25, r.Vector, r.Hybrid} {
		sf.Snapshots = append(sf.Snapshots, BenchSnapshot{
			SchemaVersion: 1,
			Mode:          m.Mode,
			RecallAt5:     m.RecallAt5,
			RecallAt10:    m.RecallAt10,
			NDCGAt5:       m.NDCGAt5,
			NDCGAt10:      m.NDCGAt10,
			MRR:           m.MRR,
			P50LatencyMs:  m.P50LatencyMs,
			P95LatencyMs:  m.P95LatencyMs,
			Timestamp:     r.Timestamp,
			CorpusHash:    corpusHash,
			CorpusName:    corpusName,
		})
	}
	return sf
}

// SaveSnapshot writes a SnapshotFile to disk in snapDir.
// Returns the full path to the saved file.
func SaveSnapshot(snapDir string, snap SnapshotFile) (string, error) {
	if err := os.MkdirAll(snapDir, 0755); err != nil {
		return "", fmt.Errorf("create snapshots dir: %w", err)
	}
	path := SnapshotFilePath(snapDir, snap.CorpusName, snap.CorpusHash)
	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("create snapshot file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(snap); err != nil {
		return "", fmt.Errorf("encode snapshot: %w", err)
	}
	return path, nil
}

// LoadSnapshot reads a SnapshotFile from disk.
func LoadSnapshot(path string) (SnapshotFile, error) {
	var snap SnapshotFile
	f, err := os.Open(path)
	if err != nil {
		return snap, fmt.Errorf("open snapshot: %w", err)
	}
	defer f.Close()

	if err := json.NewDecoder(f).Decode(&snap); err != nil {
		return snap, fmt.Errorf("decode snapshot: %w", err)
	}
	return snap, nil
}

// FindBaselineSnapshot looks for an existing snapshot matching the corpus hash.
// Returns zero-value SnapshotFile and empty path when no baseline exists.
func FindBaselineSnapshot(snapDir, corpusName, corpusHash string) (SnapshotFile, string, error) {
	path := SnapshotFilePath(snapDir, corpusName, corpusHash)
	snap, err := LoadSnapshot(path)
	if err != nil {
		if os.IsNotExist(err) {
			return SnapshotFile{}, "", nil
		}
		return SnapshotFile{}, "", err
	}
	return snap, path, nil
}

// Delta represents the change in a single metric between baseline and current results.
type Delta struct {
	Metric   string  `json:"metric"`
	Mode     string  `json:"mode"`
	Baseline float64 `json:"baseline"`
	Current  float64 `json:"current"`
	Delta    float64 `json:"delta"` // current - baseline
	Better   bool    `json:"better"`
}

// ComparisonReport holds the full comparison between baseline and current snapshots.
type ComparisonReport struct {
	Baseline SnapshotFile `json:"baseline"`
	Current  SnapshotFile `json:"current"`
	Deltas   []Delta      `json:"deltas"`
	Passed   bool         `json:"passed"`
}

// CompareSnapshots compares current results against a baseline and returns
// a ComparisonReport with per-metric deltas. A metric is "better" when it
// improves (higher recall/NDCG/MRR, lower latency).
func CompareSnapshots(baseline, current SnapshotFile) ComparisonReport {
	report := ComparisonReport{
		Baseline: baseline,
		Current:  current,
	}

	baselineByMode := make(map[string]BenchSnapshot)
	for _, s := range baseline.Snapshots {
		baselineByMode[s.Mode] = s
	}

	for _, cur := range current.Snapshots {
		base, ok := baselineByMode[cur.Mode]
		if !ok {
			// Mode not in baseline — skip (shouldn't happen with standard modes)
			continue
		}

		type metricDef struct {
			Name           string
			Baseline       float64
			Current        float64
			HigherIsBetter bool
		}
		metrics := []metricDef{
			{"Recall@5", base.RecallAt5, cur.RecallAt5, true},
			{"Recall@10", base.RecallAt10, cur.RecallAt10, true},
			{"NDCG@5", base.NDCGAt5, cur.NDCGAt5, true},
			{"NDCG@10", base.NDCGAt10, cur.NDCGAt10, true},
			{"MRR", base.MRR, cur.MRR, true},
			{"P50 Latency (ms)", base.P50LatencyMs, cur.P50LatencyMs, false},
			{"P95 Latency (ms)", base.P95LatencyMs, cur.P95LatencyMs, false},
		}

		for _, m := range metrics {
			deltaVal := m.Current - m.Baseline
			var better bool
			if m.HigherIsBetter {
				better = deltaVal >= 0
			} else {
				better = deltaVal <= 0 // latency decreased = better
			}
			report.Deltas = append(report.Deltas, Delta{
				Metric:   m.Name,
				Mode:     cur.Mode,
				Baseline: m.Baseline,
				Current:  m.Current,
				Delta:    deltaVal,
				Better:   better,
			})
		}
	}

	report.Passed = true
	for _, d := range report.Deltas {
		if !d.Better {
			report.Passed = false
			break
		}
	}

	return report
}

// WriteComparisonReport writes the comparison report to w in human-readable format.
func WriteComparisonReport(w io.Writer, report ComparisonReport) {
	fmt.Fprintf(w, "=== Benchmark Comparison ===\n\n")
	fmt.Fprintf(w, "Baseline: %s (corpus: %s, hash: %s)\n",
		report.Baseline.CreatedAt.Format(time.RFC3339),
		report.Baseline.CorpusName,
		shortHash(report.Baseline.CorpusHash))
	fmt.Fprintf(w, "Current:  %s (corpus: %s, hash: %s)\n\n",
		report.Current.CreatedAt.Format(time.RFC3339),
		report.Current.CorpusName,
		shortHash(report.Current.CorpusHash))

	if report.Passed {
		fmt.Fprintf(w, "✓ ALL METRICS PASSED — no regressions detected\n\n")
	} else {
		fmt.Fprintf(w, "✗ REGRESSIONS DETECTED\n\n")
	}

	modes := []string{"bm25", "vector", "hybrid"}
	for _, mode := range modes {
		fmt.Fprintf(w, "  [%s]\n", mode)
		hasOutput := false
		for _, d := range report.Deltas {
			if d.Mode != mode {
				continue
			}
			hasOutput = true
			arrow := "↑"
			if !d.Better {
				arrow = "↓"
			}
			status := "✓"
			if !d.Better {
				status = "✗ REGRESSION"
			}
			fmt.Fprintf(w, "    %-20s  baseline=%.4f  current=%.4f  Δ%+9.4f  %s  %s\n",
				d.Metric, d.Baseline, d.Current, d.Delta, arrow, status)
		}
		if !hasOutput {
			fmt.Fprintf(w, "    (no comparable metrics)\n")
		}
		fmt.Fprintf(w, "\n")
	}

	if !report.Passed {
		fmt.Fprintf(w, "Summary: REGRESSIONS FOUND — review the deltas above.\n")
	} else {
		fmt.Fprintf(w, "Summary: All metrics passed — performance is stable.\n")
	}
}

// shortHash returns the first 12 characters of a hex hash.
func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

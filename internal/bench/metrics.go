package bench

import (
	"math"
	"sort"
	"time"
)

// RetrievalMetrics holds the computed quality metrics for a single retrieval mode.
type RetrievalMetrics struct {
	Mode          string  `json:"mode"` // "bm25", "vector", "hybrid"
	RecallAt5     float64 `json:"recall_at_5"`
	RecallAt10    float64 `json:"recall_at_10"`
	NDCGAt5       float64 `json:"ndcg_at_5"`
	NDCGAt10      float64 `json:"ndcg_at_10"`
	MRR           float64 `json:"mrr"`
	MeanLatencyMs float64 `json:"mean_latency_ms"`
	P50LatencyMs  float64 `json:"p50_latency_ms"`
	P95LatencyMs  float64 `json:"p95_latency_ms"`
	QueryCount    int     `json:"query_count"`
	ValidFraction float64 `json:"valid_fraction,omitempty"` // temporal validity slice
	ScopeFraction float64 `json:"scope_fraction,omitempty"` // scope isolation slice
}

// RecallAtK computes the fraction of relevant documents that appear in the top-k results.
// relevant is the set of ground-truth relevant IDs; retrieved is the ordered list of result IDs.
func RecallAtK(retrieved []string, relevant map[string]bool, k int) float64 {
	if len(relevant) == 0 || k == 0 {
		return 0
	}
	if k > len(retrieved) {
		k = len(retrieved)
	}
	found := 0
	for i := 0; i < k; i++ {
		if relevant[retrieved[i]] {
			found++
		}
	}
	return float64(found) / float64(len(relevant))
}

// NDCGAtK computes Normalized Discounted Cumulative Gain at rank k.
// relevanceScores maps document IDs to binary relevance (1 = relevant, 0 = not).
// retrieved is the ordered list of result IDs from the retrieval system.
func NDCGAtK(retrieved []string, relevanceScores map[string]int, k int) float64 {
	if len(relevanceScores) == 0 || k == 0 {
		return 0
	}
	if k > len(retrieved) {
		k = len(retrieved)
	}

	// Compute DCG
	dcg := 0.0
	for i := 0; i < k; i++ {
		if rel, ok := relevanceScores[retrieved[i]]; ok && rel > 0 {
			dcg += float64(rel) / math.Log2(float64(i+2)) // i+2 because log2(1) = 0
		}
	}

	// Compute ideal DCG
	idealRels := make([]int, 0, len(relevanceScores))
	for _, rel := range relevanceScores {
		if rel > 0 {
			idealRels = append(idealRels, rel)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(idealRels)))

	idcg := 0.0
	for i := 0; i < k && i < len(idealRels); i++ {
		idcg += float64(idealRels[i]) / math.Log2(float64(i+2))
	}

	if idcg == 0 {
		return 0
	}
	return dcg / idcg
}

// MRR computes Mean Reciprocal Rank: the average of 1/rank for each query
// where rank is the position of the first relevant result.
func MRR(retrieved []string, relevant map[string]bool) float64 {
	if len(relevant) == 0 {
		return 0
	}
	for i, id := range retrieved {
		if relevant[id] {
			return 1.0 / float64(i+1)
		}
	}
	return 0
}

// LatencyPercentiles computes P50 and P95 from a slice of durations.
func LatencyPercentiles(durations []time.Duration) (p50, p95 time.Duration) {
	if len(durations) == 0 {
		return 0, 0
	}
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	p50Idx := int(math.Ceil(0.50*float64(len(sorted)))) - 1
	p95Idx := int(math.Ceil(0.95*float64(len(sorted)))) - 1
	if p50Idx < 0 {
		p50Idx = 0
	}
	if p95Idx < 0 {
		p95Idx = 0
	}
	if p50Idx >= len(sorted) {
		p50Idx = len(sorted) - 1
	}
	if p95Idx >= len(sorted) {
		p95Idx = len(sorted) - 1
	}
	return sorted[p50Idx], sorted[p95Idx]
}

// ComputeMetrics computes aggregate retrieval metrics across all queries for a single mode.
// queryResults maps query index to ordered list of retrieved memory IDs.
// groundTruth is the ordered slice of queries with their relevant ID sets.
func ComputeMetrics(mode string, queryResults map[int][]string, groundTruth []GroundTruth, latencies []time.Duration) RetrievalMetrics {
	var recall5Sum, recall10Sum, ndcg5Sum, ndcg10Sum, mrrSum float64
	validCount := 0

	for i, gt := range groundTruth {
		retrieved := queryResults[i]
		relevant := make(map[string]bool)
		for _, id := range gt.RelevantIDs {
			relevant[id] = true
		}

		// Binary relevance for NDCG
		relevanceScores := make(map[string]int)
		for _, id := range gt.RelevantIDs {
			relevanceScores[id] = 1
		}

		recall5Sum += RecallAtK(retrieved, relevant, 5)
		recall10Sum += RecallAtK(retrieved, relevant, 10)
		ndcg5Sum += NDCGAtK(retrieved, relevanceScores, 5)
		ndcg10Sum += NDCGAtK(retrieved, relevanceScores, 10)
		mrrSum += MRR(retrieved, relevant)
		validCount++
	}

	n := float64(validCount)
	if n == 0 {
		n = 1
	}

	var meanLat, p50, p95 float64
	if len(latencies) > 0 {
		p50d, p95d := LatencyPercentiles(latencies)
		p50 = float64(p50d.Microseconds()) / 1000.0
		p95 = float64(p95d.Microseconds()) / 1000.0
		var total time.Duration
		for _, d := range latencies {
			total += d
		}
		meanLat = float64(total.Microseconds()) / float64(len(latencies)) / 1000.0
	}

	return RetrievalMetrics{
		Mode:          mode,
		RecallAt5:     recall5Sum / n,
		RecallAt10:    recall10Sum / n,
		NDCGAt5:       ndcg5Sum / n,
		NDCGAt10:      ndcg10Sum / n,
		MRR:           mrrSum / n,
		MeanLatencyMs: meanLat,
		P50LatencyMs:  p50,
		P95LatencyMs:  p95,
		QueryCount:    validCount,
	}
}

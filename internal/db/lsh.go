package db

import "math/rand"

const (
	// LSHBits controls the number of hash bits (and thus bucket count = 2^LSHBits).
	// 16 bits → 65,536 buckets for good recall on datasets up to ~100K memories.
	LSHBits = 16
	// EmbeddingDim is the expected vector dimensionality.
	EmbeddingDim = 768
)

// Fixed-seed random projection vectors for deterministic LSH across restarts.
var lshProjections [][]float32

func init() {
	rng := rand.New(rand.NewSource(42))
	lshProjections = make([][]float32, LSHBits)
	for i := range lshProjections {
		lshProjections[i] = make([]float32, EmbeddingDim)
		for j := range lshProjections[i] {
			// Random normal approximation via Box-Muller
			lshProjections[i][j] = float32(rng.NormFloat64())
		}
	}
}

// ComputeLSH computes the LSH hash bits for a vector.
// Returns an integer where each bit represents the sign of the dot product
// with one of the fixed random projection vectors.
func ComputeLSH(vec []float32) int {
	if len(vec) == 0 {
		return 0
	}
	var hash int
	for i, proj := range lshProjections {
		var dot float64
		lim := min(len(vec), len(proj))
		for j := range lim {
			dot += float64(vec[j] * proj[j])
		}
		if dot >= 0 {
			hash |= 1 << i
		}
	}
	return hash
}

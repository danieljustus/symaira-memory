package extractor

import (
	"testing"
)

func TestLocalHashVectorizer(t *testing.T) {
	dims := 768

	text1 := "User prefers typescript"
	text2 := "User prefers typescript" // identical
	text3 := "Different context statement"

	vec1 := GenerateLocalHashVector(text1, dims)
	vec2 := GenerateLocalHashVector(text2, dims)
	vec3 := GenerateLocalHashVector(text3, dims)

	// Test dimensions
	if len(vec1) != dims {
		t.Errorf("expected vector size %d, got %d", dims, len(vec1))
	}

	// Test determinism
	for i := 0; i < dims; i++ {
		if vec1[i] != vec2[i] {
			t.Errorf("vectorizer is not deterministic: mismatch at dimension %d", i)
		}
	}

	// Test L2 Normalization (sum of squares must be 1.0)
	var sumSquares float64
	for _, val := range vec1 {
		sumSquares += float64(val * val)
	}

	// Margin check (float precision delta)
	if sumSquares > 0.0 && mathAbs(float32(sumSquares)-1.0) > 1e-5 {
		t.Errorf("vector is not L2 normalized: sum of squares is %f", sumSquares)
	}

	// Test distinct contexts have different vector representations
	matches := 0
	for i := 0; i < dims; i++ {
		if vec1[i] == vec3[i] && vec1[i] != 0 {
			matches++
		}
	}
	if matches == dims {
		t.Errorf("distinct statements produced identical vectors")
	}
}

func mathAbs(f float32) float32 {
	if f < 0 {
		return -f
	}
	return f
}

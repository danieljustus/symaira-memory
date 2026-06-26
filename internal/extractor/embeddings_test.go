package extractor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

func TestActiveBackendDefaultOllama(t *testing.T) {
	eg := NewEmbeddingsGenerator(nil)
	if got := eg.ActiveBackend(); got != "ollama" {
		t.Errorf("expected ActiveBackend() to return 'ollama' on fresh instance, got %q", got)
	}
}

func TestActiveBackendLexicalAfterFailure(t *testing.T) {
	eg := NewEmbeddingsGenerator(nil)
	eg.mu.Lock()
	eg.lastFail = time.Now()
	eg.mu.Unlock()
	if got := eg.ActiveBackend(); got != "lexical" {
		t.Errorf("expected ActiveBackend() to return 'lexical' after recent failure, got %q", got)
	}
}

func TestActiveBackendRecoveryAfterCooldown(t *testing.T) {
	eg := NewEmbeddingsGenerator(nil)
	eg.mu.Lock()
	eg.lastFail = time.Now().Add(-ollamaCacheTTL - time.Second)
	eg.mu.Unlock()
	if got := eg.ActiveBackend(); got != "ollama" {
		t.Errorf("expected ActiveBackend() to return 'ollama' after cooldown, got %q", got)
	}
}

func TestGenerateVectorTimeoutFallsBack(t *testing.T) {
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer slowServer.Close()

	eg := NewEmbeddingsGenerator(nil)
	eg.OllamaURL = slowServer.URL
	eg.OllamaTimeout = 50 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	result := eg.GenerateVectorWithContext(ctx, "test query")
	elapsed := time.Since(start)

	if result.Source != "hash-fallback" {
		t.Errorf("expected hash-fallback on timeout, got %q", result.Source)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("timeout fallback took too long: %v", elapsed)
	}
}

func TestGenerateVectorContextCanceledFallsBack(t *testing.T) {
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer slowServer.Close()

	eg := NewEmbeddingsGenerator(nil)
	eg.OllamaURL = slowServer.URL
	eg.OllamaTimeout = 5 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := eg.GenerateVectorWithContext(ctx, "test query")

	if result.Source != "hash-fallback" {
		t.Errorf("expected hash-fallback on canceled context, got %q", result.Source)
	}
}

func TestMetricsTracking(t *testing.T) {
	eg := NewEmbeddingsGenerator(nil)
	eg.MarkOllamaFailed()

	for i := 0; i < 5; i++ {
		eg.GenerateVector("test query")
	}

	metrics := eg.Metrics()
	if metrics.TotalRequests != 5 {
		t.Errorf("expected 5 total requests, got %d", metrics.TotalRequests)
	}
	if metrics.FallbackCount != 5 {
		t.Errorf("expected 5 fallbacks, got %d", metrics.FallbackCount)
	}
	if metrics.FallbackRate != 1.0 {
		t.Errorf("expected fallback rate 1.0, got %f", metrics.FallbackRate)
	}
}

func mathAbs(f float32) float32 {
	if f < 0 {
		return -f
	}
	return f
}

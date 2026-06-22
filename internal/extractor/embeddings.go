package extractor

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/danieljustus/symaira-memory/internal/config"
	lru "github.com/hashicorp/golang-lru/v2"
)

// EmbeddingsGenerator coordinates local and cloud-fallback embedding generation.
type EmbeddingsGenerator struct {
	OllamaURL      string
	Model          string
	httpClient     *http.Client
	mu             sync.Mutex
	lastFail       time.Time
	embeddingCache *lru.Cache[string, []float32]
}

const (
	ollamaCacheTTL = 30 * time.Second
	defaultTimeout = 5 * time.Second
)

// NewEmbeddingsGenerator configures an embeddings generator from the
// supplied config. The caller (typically cmd/) is responsible for
// loading configuration via config.Load(); this package never reads
// config files directly. When cfg is nil, hardcoded defaults are used.
func NewEmbeddingsGenerator(cfg *config.Config) *EmbeddingsGenerator {
	if cfg == nil {
		cfg = config.Defaults()
	}
	ollamaURL := "http://localhost:11434/api/embeddings"
	model := "nomic-embed-text"
	if cfg.Ollama.URL != "" {
		ollamaURL = cfg.Ollama.URL
	}
	if cfg.Ollama.Model != "" {
		model = cfg.Ollama.Model
	}

	cache, _ := lru.New[string, []float32](10000)

	return &EmbeddingsGenerator{
		OllamaURL: ollamaURL,
		Model:     model,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
			Transport: &http.Transport{
				MaxIdleConns:    10,
				IdleConnTimeout: 90 * time.Second,
			},
		},
		embeddingCache: cache,
	}
}

// EmbeddingResult carries the generated vector together with provenance
// metadata that identifies which embedding space the vector belongs to.
// Search and consolidation must never cross-score rows from different sources.
type EmbeddingResult struct {
	Vector []float32 // the embedding vector
	Source string    // "ollama" or "hash-fallback"
	Model  string    // model name (e.g. "nomic-embed-text") or "" for hash
}

// GenerateVector produces a 768-dimensional vector using Ollama if available,
// or the local hashing fallback. Ollama vectors are cached by content hash to
// avoid redundant computation for identical text. Fallback vectors are never
// cached so that recovery after Ollama comes back online is automatic.
func (eg *EmbeddingsGenerator) GenerateVector(text string) EmbeddingResult {
	cacheKey := eg.cacheKey(text)
	if cached, ok := eg.embeddingCache.Get(cacheKey); ok {
		return EmbeddingResult{Vector: cached, Source: "ollama", Model: eg.Model}
	}

	dims := 768

	eg.mu.Lock()
	skip := time.Since(eg.lastFail) < ollamaCacheTTL
	eg.mu.Unlock()

	if !skip {
		vec, err := eg.queryOllama(text)
		if err == nil && len(vec) == dims {
			eg.embeddingCache.Add(cacheKey, vec)
			return EmbeddingResult{Vector: vec, Source: "ollama", Model: eg.Model}
		}
		eg.mu.Lock()
		eg.lastFail = time.Now()
		eg.mu.Unlock()
	}

	// Fallback vectors are intentionally NOT cached so that when Ollama
	// recovers, the next request for the same text will succeed via Ollama
	// and produce a properly cached Ollama vector.
	vec := GenerateLocalHashVector(text, dims)
	return EmbeddingResult{Vector: vec, Source: "hash-fallback", Model: ""}
}

// ActiveBackend reports the embedding backend that would be used for the
// next GenerateVector call. It returns "ollama" when Ollama is reachable or
// the cooldown has expired, and "lexical" when Ollama is currently being
// skipped due to a recent failure within the cooldown window.
func (eg *EmbeddingsGenerator) ActiveBackend() string {
	eg.mu.Lock()
	defer eg.mu.Unlock()
	if time.Since(eg.lastFail) < ollamaCacheTTL {
		return "lexical"
	}
	return "ollama"
}

// MarkOllamaFailed records an Ollama failure, switching the generator to
// lexical-fallback mode for the duration of the cooldown window. This is
// useful in tests that need to exercise the fallback path.
func (eg *EmbeddingsGenerator) MarkOllamaFailed() {
	eg.mu.Lock()
	eg.lastFail = time.Now()
	eg.mu.Unlock()
}

func (eg *EmbeddingsGenerator) cacheKey(text string) string {
	h := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", h[:16])
}

func (eg *EmbeddingsGenerator) queryOllama(text string) ([]float32, error) {
	reqBody, err := json.Marshal(map[string]string{
		"model":  eg.Model,
		"prompt": text,
	})
	if err != nil {
		return nil, err
	}

	resp, err := eg.httpClient.Post(eg.OllamaURL, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	var res struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	return res.Embedding, nil
}

// GenerateLocalHashVector utilizes the "Hashing Trick" to produce a normalized 768-dim vector in microseconds.
func GenerateLocalHashVector(text string, dimensions int) []float32 {
	vec := make([]float32, dimensions)

	// Normalize and tokenize text
	cleaned := strings.ToLower(text)
	// Replace punctuation with spaces
	for _, char := range []string{".", ",", "!", "?", ";", ":", "-", "_", "(", ")", "[", "]", "{", "}"} {
		cleaned = strings.ReplaceAll(cleaned, char, " ")
	}

	words := strings.Fields(cleaned)
	if len(words) == 0 {
		return vec
	}

	// Distribute word hashes across vector dimensions
	for _, word := range words {
		if isStopWord(word) {
			continue
		}

		h := fnv.New32a()
		h.Write([]byte(word))
		hashVal := h.Sum32()

		idx := int(hashVal) % dimensions

		// Add weighting based on hash signature
		vec[idx] += 1.0
	}

	// Normalize the vector (L2 norm) so cosine similarity behaves correctly
	var sumSquares float64
	for _, val := range vec {
		sumSquares += float64(val * val)
	}

	if sumSquares > 0 {
		norm := float32(math.Sqrt(sumSquares))
		for i := range vec {
			vec[i] /= norm
		}
	}

	return vec
}

func isStopWord(w string) bool {
	// Standard compact list of English and German stop words
	stops := map[string]bool{
		"and": true, "the": true, "a": true, "an": true, "of": true, "to": true, "in": true, "is": true, "it": true, "that": true,
		"und": true, "der": true, "die": true, "das": true, "ein": true, "eine": true, "ist": true, "es": true, "dass": true,
		"von": true, "zu": true, "mit": true, "auf": true, "für": true, "den": true, "dem": true, "des": true, "im": true, "am": true,
	}
	return stops[w]
}

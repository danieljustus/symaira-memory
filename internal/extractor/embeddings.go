package extractor

import (
	"bytes"
	"encoding/json"
	"hash/fnv"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

// EmbeddingsGenerator coordinates local and cloud-fallback embedding generation.
type EmbeddingsGenerator struct {
	OllamaURL     string
	Model         string
	httpClient    *http.Client
	mu            sync.Mutex
	lastFail      time.Time
}

const (
	ollamaCacheTTL = 30 * time.Second
	defaultTimeout = 5 * time.Second
)

// NewEmbeddingsGenerator sets up standard configuration with a shared, pooled HTTP client.
func NewEmbeddingsGenerator() *EmbeddingsGenerator {
	return &EmbeddingsGenerator{
		OllamaURL: "http://localhost:11434/api/embeddings",
		Model:     "nomic-embed-text",
		httpClient: &http.Client{
			Timeout: defaultTimeout,
			Transport: &http.Transport{
				MaxIdleConns:    10,
				IdleConnTimeout: 90 * time.Second,
			},
		},
	}
}

// GenerateVector produces a 768-dimensional vector using Ollama if available, or the local hashing fallback.
func (eg *EmbeddingsGenerator) GenerateVector(text string) []float32 {
	dims := 768

	eg.mu.Lock()
	skip := time.Since(eg.lastFail) < ollamaCacheTTL
	eg.mu.Unlock()

	if !skip {
		vec, err := eg.queryOllama(text)
		if err == nil && len(vec) == dims {
			return vec
		}
		eg.mu.Lock()
		eg.lastFail = time.Now()
		eg.mu.Unlock()
	}

	return GenerateLocalHashVector(text, dims)
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
		return nil, err
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

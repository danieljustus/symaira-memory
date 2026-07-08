package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/danieljustus/symaira-corekit/ollamakit"
)

type Client struct {
	OllamaURL   string
	OllamaModel string
	HTTPClient  *http.Client
	ollama      *ollamakit.Client
}

func NewClient(ollamaURL, ollamaModel string) *Client {
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434/api/generate"
	}
	if ollamaModel == "" {
		ollamaModel = "llama3"
	}
	c := &Client{
		OllamaURL:   ollamaURL,
		OllamaModel: ollamaModel,
		HTTPClient: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
	c.ollama = ollamakit.New(ollamakit.Config{
		BaseURL: ollamaBaseURL(ollamaURL),
		Model:   ollamaModel,
		Timeout: 45 * time.Second,
	})
	return c
}

// ollamaBaseURL strips a configured Ollama endpoint path (e.g.
// "http://localhost:11434/api/generate") down to the scheme+host root
// ollamakit.Config.BaseURL expects. Malformed input is passed through
// unchanged so ollamakit's own defaulting takes over.
func ollamaBaseURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return raw
	}
	return u.Scheme + "://" + u.Host
}

func (c *Client) QueryOllama(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	var out strings.Builder
	err := c.ollama.Generate(ctx, c.OllamaModel, userPrompt, &ollamakit.GenerateOptions{
		Format: "json",
		System: systemPrompt,
	}, func(chunk ollamakit.GenerateResponse) error {
		out.WriteString(chunk.Response)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to query Ollama: %w", err)
	}
	return out.String(), nil
}

func (c *Client) QueryOpenAI(ctx context.Context, systemPrompt, userPrompt, apiKey, model, url string) (string, error) {
	if model == "" {
		model = "gpt-4o-mini"
	}
	if url == "" {
		url = "https://api.openai.com/v1/chat/completions"
	}

	reqBody, err := json.Marshal(map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"response_format": map[string]string{"type": "json_object"},
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to query OpenAI: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai returned HTTP status %d", resp.StatusCode)
	}

	var res struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}

	if len(res.Choices) == 0 {
		return "", fmt.Errorf("openai returned empty choices")
	}

	return res.Choices[0].Message.Content, nil
}

func (c *Client) Query(ctx context.Context, systemPrompt, userPrompt, provider, apiKey string) (string, error) {
	if provider == "openai" {
		if apiKey == "" {
			apiKey = os.Getenv("OPENAI_API_KEY")
		}
		if apiKey == "" {
			return "", fmt.Errorf("OPENAI_API_KEY environment variable is not set")
		}
		return c.QueryOpenAI(ctx, systemPrompt, userPrompt, apiKey, "", "")
	}
	return c.QueryOllama(ctx, systemPrompt, userPrompt)
}

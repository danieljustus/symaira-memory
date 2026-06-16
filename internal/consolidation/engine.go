package consolidation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/danieljustus/symaira-memory/internal/security"
	"github.com/google/uuid"
)

// ConsolidationResult represents the parsed response from the LLM.
type ConsolidationResult struct {
	Consolidated []ConsolidatedItem `json:"consolidated"`
	DiscardedIDs []string           `json:"discarded_ids"`
}

// ConsolidatedItem represents a newly synthesized memory fact.
type ConsolidatedItem struct {
	Content     string            `json:"content"`
	ReplacesIDs []string          `json:"replaces_ids"`
	Metadata    map[string]string `json:"metadata"`
}

// ScopeChangeSummary captures the actions taken or planned for a single scope.
type ScopeChangeSummary struct {
	Scope             string
	NewMemories       []*db.Memory
	ArchivedMemoryIDs []string
	ReplacedIDToNewID map[string]string
}

// Engine orchestrates the memory consolidation process.
type Engine struct {
	database    *db.DB
	embeddings  *extractor.EmbeddingsGenerator
	llmURL      string
	llmModel    string
	llmProvider string // "ollama" | "openai"
	piiEnabled  bool
	httpClient  *http.Client
}

// NewEngine creates a new consolidation engine instance.
func NewEngine(database *db.DB, embeddings *extractor.EmbeddingsGenerator, llmURL, llmModel, llmProvider string, piiEnabled bool) *Engine {
	if llmURL == "" {
		if llmProvider == "openai" {
			llmURL = "https://api.openai.com/v1/chat/completions"
		} else {
			llmURL = "http://localhost:11434/api/generate"
		}
	}
	if llmModel == "" {
		if llmProvider == "openai" {
			llmModel = "gpt-4o-mini"
		} else {
			llmModel = "llama3"
		}
	}
	if llmProvider == "" {
		if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
			llmProvider = "openai"
		} else {
			llmProvider = "ollama"
		}
	}

	return &Engine{
		database:    database,
		embeddings:  embeddings,
		llmURL:      llmURL,
		llmModel:    llmModel,
		llmProvider: llmProvider,
		piiEnabled:  piiEnabled,
		httpClient: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

// RunConsolidation finds raw memories, groups them by scope, prompts the LLM,
// and applies the changes inside a database transaction (unless dryRun is true).
func (eng *Engine) RunConsolidation(ctx context.Context, scopeFilter string, dryRun bool) ([]ScopeChangeSummary, error) {
	rawMemories, err := eng.database.GetRawMemories()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch raw memories: %w", err)
	}

	if len(rawMemories) == 0 {
		return nil, nil
	}

	// Group raw memories by scope
	grouped := make(map[string][]*db.Memory)
	for _, m := range rawMemories {
		if scopeFilter != "" && m.Scope != scopeFilter {
			continue
		}
		grouped[m.Scope] = append(grouped[m.Scope], m)
	}

	var summaries []ScopeChangeSummary

	for scope, memories := range grouped {
		summary := ScopeChangeSummary{
			Scope:             scope,
			ReplacedIDToNewID: make(map[string]string),
		}

		// If there is only one raw memory, we don't need LLM consolidation,
		// we can simply mark it as consolidated.
		if len(memories) <= 1 {
			m := memories[0]
			updatedMemory := *m
			updatedMemory.ConsolidationStatus = "consolidated"
			updatedMemory.UpdatedAt = time.Now().UTC()

			summary.NewMemories = append(summary.NewMemories, &updatedMemory)
			summary.ArchivedMemoryIDs = append(summary.ArchivedMemoryIDs, m.ID)
			summary.ReplacedIDToNewID[m.ID] = m.ID

			if !dryRun {
				tx, err := eng.database.BeginTransaction()
				if err != nil {
					return nil, fmt.Errorf("failed to begin transaction: %w", err)
				}
				if err := eng.database.SaveMemoryTx(tx, &updatedMemory); err != nil {
					tx.Rollback()
					return nil, fmt.Errorf("failed to update raw memory to consolidated: %w", err)
				}
				if err := tx.Commit(); err != nil {
					return nil, fmt.Errorf("failed to commit transaction: %w", err)
				}
			}
			summaries = append(summaries, summary)
			continue
		}

		// Prompt the LLM for consolidation
		res, err := eng.consolidateWithLLM(ctx, scope, memories)
		if err != nil {
			// If LLM fails, we log it and skip this scope, allowing subsequent scopes to proceed
			// or fail gracefully. For now, return error to caller.
			return nil, fmt.Errorf("llm consolidation failed for scope %s: %w", scope, err)
		}

		// Process results
		txMemMap := make(map[string]*db.Memory)

		for _, item := range res.Consolidated {
			content := item.Content
			if eng.piiEnabled {
				content = security.Redact(content)
			}

			// Generate vector embedding
			var vector []float32
			if eng.embeddings != nil {
				vector = eng.embeddings.GenerateVector(content)
			}

			newID := uuid.New().String()
			now := time.Now().UTC()

			meta := item.Metadata
			if meta == nil {
				meta = make(map[string]string)
			}

			newMem := &db.Memory{
				ID:                  newID,
				Content:             content,
				Scope:               scope,
				Metadata:            meta,
				Embedding:           vector,
				CreatedAt:           now,
				UpdatedAt:           now,
				ConsolidationStatus: "consolidated",
			}

			summary.NewMemories = append(summary.NewMemories, newMem)
			txMemMap[newID] = newMem

			for _, replID := range item.ReplacesIDs {
				summary.ArchivedMemoryIDs = append(summary.ArchivedMemoryIDs, replID)
				summary.ReplacedIDToNewID[replID] = newID
			}
		}

		for _, discID := range res.DiscardedIDs {
			summary.ArchivedMemoryIDs = append(summary.ArchivedMemoryIDs, discID)
		}

		if !dryRun {
			tx, err := eng.database.BeginTransaction()
			if err != nil {
				return nil, fmt.Errorf("failed to begin transaction: %w", err)
			}

			// Save new memories
			for _, m := range summary.NewMemories {
				if err := eng.database.SaveMemoryTx(tx, m); err != nil {
					tx.Rollback()
					return nil, fmt.Errorf("failed to save consolidated memory: %w", err)
				}
			}

			// Update replaced memories to archived with parent link
			for _, m := range memories {
				status := "archived"
				parentID := summary.ReplacedIDToNewID[m.ID]

				isDiscarded := false
				for _, discID := range res.DiscardedIDs {
					if discID == m.ID {
						isDiscarded = true
						break
					}
				}

				isReplaced := parentID != ""

				// If not explicitly replaced or discarded by LLM, let's keep it raw
				// to avoid losing data due to LLM parsing/omission issues.
				if !isReplaced && !isDiscarded {
					continue
				}

				if isDiscarded {
					parentID = ""
				}

				if err := eng.database.UpdateMemoryStatusTx(tx, m.ID, status, parentID); err != nil {
					tx.Rollback()
					return nil, fmt.Errorf("failed to archive original memory %s: %w", m.ID, err)
				}
			}

			if err := tx.Commit(); err != nil {
				return nil, fmt.Errorf("failed to commit transaction: %w", err)
			}
		}

		summaries = append(summaries, summary)
	}

	return summaries, nil
}

func (eng *Engine) consolidateWithLLM(ctx context.Context, scope string, memories []*db.Memory) (*ConsolidationResult, error) {
	var builder strings.Builder
	builder.WriteString("Raw Memories:\n")
	for _, m := range memories {
		builder.WriteString(fmt.Sprintf("- ID: %s\n  Content: %s\n  Created: %s\n", m.ID, m.Content, m.CreatedAt.Format(time.RFC3339)))
	}

	prompt := fmt.Sprintf(`You are the Symaira Memory Consolidation Engine.
Your task is to analyze and consolidate raw, new memories for scope: "%s".
Follow these rules:
1. Merge duplicate or highly similar memories into a single concise fact.
2. Resolve contradictory facts, prioritizing the most recent information based on the timestamps.
3. Identify purely temporary or transient memories (e.g., "going to lunch", "looking for coffee") and list their IDs under "discarded_ids".
4. For consolidated items, list the IDs of the original memories that were merged into it in "replaces_ids".
5. Do not include any greeting, explanation, or markdown backticks in your response. Output ONLY valid JSON matching the schema below.

JSON Schema:
{
  "consolidated": [
    {
      "content": "Synthesized fact (written in third person, e.g., 'Daniel prefers dark mode.')",
      "replaces_ids": ["id1", "id2"],
      "metadata": { "topic": "preferences" }
    }
  ],
  "discarded_ids": ["id3"]
}

%s`, scope, builder.String())

	var rawResponse string
	var err error

	if eng.llmProvider == "openai" {
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY environment variable is not set")
		}
		rawResponse, err = eng.queryOpenAI(ctx, prompt, apiKey)
	} else {
		rawResponse, err = eng.queryOllama(ctx, prompt)
	}

	if err != nil {
		return nil, err
	}

	return parseJSONResponse(rawResponse)
}

func (eng *Engine) queryOllama(ctx context.Context, prompt string) (string, error) {
	reqBody, err := json.Marshal(map[string]interface{}{
		"model":  eng.llmModel,
		"prompt": prompt,
		"stream": false,
		"format": "json",
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", eng.llmURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := eng.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to query Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama returned HTTP status %d", resp.StatusCode)
	}

	var res struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}

	return res.Response, nil
}

func (eng *Engine) queryOpenAI(ctx context.Context, prompt string, apiKey string) (string, error) {
	reqBody, err := json.Marshal(map[string]interface{}{
		"model": eng.llmModel,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"response_format": map[string]string{"type": "json_object"},
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", eng.llmURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := eng.httpClient.Do(req)
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

func parseJSONResponse(rawResponse string) (*ConsolidationResult, error) {
	cleaned := strings.TrimSpace(rawResponse)
	if strings.HasPrefix(cleaned, "```") {
		lines := strings.Split(cleaned, "\n")
		if len(lines) > 2 {
			if strings.HasPrefix(lines[0], "```") {
				lines = lines[1:]
			}
			if strings.HasSuffix(lines[len(lines)-1], "```") {
				lines = lines[:len(lines)-1]
			}
			cleaned = strings.Join(lines, "\n")
		}
	}
	cleaned = strings.TrimSpace(cleaned)

	var res ConsolidationResult
	if err := json.Unmarshal([]byte(cleaned), &res); err != nil {
		return nil, fmt.Errorf("failed to parse consolidation result JSON: %w (raw response: %s)", err, rawResponse)
	}
	return &res, nil
}

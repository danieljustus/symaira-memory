package contextassembler

import (
	"fmt"
	"strings"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/danieljustus/symaira-memory/internal/summarizer"
)

type ContextLayer string

const (
	LayerWorkingContext ContextLayer = "working_context"
	LayerSummary        ContextLayer = "summary"
	LayerRetrieval      ContextLayer = "retrieval"
)

type AssembledPiece struct {
	Layer   ContextLayer `json:"layer"`
	Content string       `json:"content"`
	Tokens  int          `json:"tokens"`
}

type AssembledContext struct {
	Query      string           `json:"query"`
	Budget     int              `json:"budget"`
	UsedTokens int              `json:"used_tokens"`
	Pieces     []AssembledPiece `json:"pieces"`
}

type Assembler struct {
	database   *db.DB
	summarizer *summarizer.ExtractiveSummarizer
	embeddings *extractor.EmbeddingsGenerator
	cfg        *config.ContextConfig
}

func NewAssembler(database *db.DB, embeddings *extractor.EmbeddingsGenerator, cfg *config.ContextConfig) *Assembler {
	if cfg == nil {
		defaults := config.Defaults()
		cfg = &defaults.Context
	}
	return &Assembler{
		database:   database,
		summarizer: summarizer.NewExtractiveSummarizer(),
		embeddings: embeddings,
		cfg:        cfg,
	}
}

func (a *Assembler) Assemble(query string, sessionText string, sessionID string) (*AssembledContext, error) {
	budget := a.cfg.TokenBudget
	if budget <= 0 {
		budget = 2000
	}
	ctx := &AssembledContext{
		Query:  query,
		Budget: budget,
	}

	usedTokens := 0

	if sessionText != "" && a.cfg.MaxWorkingTurns > 0 {
		workingCtx := extractWorkingContext(sessionText, a.cfg.MaxWorkingTurns)
		workingTokens := estimateTokens(workingCtx)
		if usedTokens+workingTokens <= budget {
			ctx.Pieces = append(ctx.Pieces, AssembledPiece{
				Layer:   LayerWorkingContext,
				Content: workingCtx,
				Tokens:  workingTokens,
			})
			usedTokens += workingTokens
		}
	}

	if sessionID != "" {
		summary, err := a.database.GetSessionSummary(sessionID)
		if err == nil && summary != "" {
			summaryTokens := estimateTokens(summary)
			if usedTokens+summaryTokens <= budget {
				ctx.Pieces = append(ctx.Pieces, AssembledPiece{
					Layer:   LayerSummary,
					Content: summary,
					Tokens:  summaryTokens,
				})
				usedTokens += summaryTokens
			}
		}
	}

	if query != "" && usedTokens < budget {
		remaining := budget - usedTokens
		retrievalResults, err := a.retrieveRelevant(query, remaining)
		if err == nil && len(retrievalResults) > 0 {
			retrievalContent := formatRetrievalResults(retrievalResults)
			retrievalTokens := estimateTokens(retrievalContent)
			if usedTokens+retrievalTokens <= budget {
				ctx.Pieces = append(ctx.Pieces, AssembledPiece{
					Layer:   LayerRetrieval,
					Content: retrievalContent,
					Tokens:  retrievalTokens,
				})
				usedTokens += retrievalTokens
			}
		}
	}

	ctx.UsedTokens = usedTokens
	return ctx, nil
}

func (a *Assembler) ProduceSessionSummary(sessionText string, sessionID string) (string, error) {
	if sessionText == "" {
		return "", nil
	}
	summary := a.summarizer.SummarizeSession(sessionText, 5)
	if err := a.database.SaveSessionSummary(sessionID, summary); err != nil {
		return "", fmt.Errorf("failed to save session summary: %w", err)
	}
	return summary, nil
}

func (a *Assembler) retrieveRelevant(query string, tokenBudget int) ([]db.SearchResult, error) {
	if a.embeddings == nil {
		return nil, nil
	}
	emb := a.embeddings.GenerateVector(query)
	queryVec := emb.Vector
	limit := tokenBudget / 50
	if limit < 1 {
		limit = 1
	}
	if limit > 20 {
		limit = 20
	}
	return a.database.SearchMemories(queryVec, emb.Source, "", limit)
}

func extractWorkingContext(sessionText string, maxTurns int) string {
	lines := strings.Split(sessionText, "\n")
	if len(lines) <= maxTurns*2 {
		return sessionText
	}
	start := len(lines) - maxTurns*2
	if start < 0 {
		start = 0
	}
	return strings.Join(lines[start:], "\n")
}

func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	words := strings.Fields(text)
	return (len(words) * 4) / 3
}

func formatRetrievalResults(results []db.SearchResult) string {
	var sb strings.Builder
	sb.WriteString("Relevant stored memories:\n")
	for i, r := range results {
		if i >= 10 {
			break
		}
		fmt.Fprintf(&sb, "- %s\n", r.Memory.Content)
	}
	return sb.String()
}

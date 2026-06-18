package contextassembler

import (
	"strings"
	"testing"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
)

func TestExtractWorkingContext_Trimmed(t *testing.T) {
	text := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\nLine 6\nLine 7\nLine 8\nLine 9\nLine 10"
	result := extractWorkingContext(text, 2)
	lines := 0
	for range []byte(result) {
		if result[lines] == '\n' {
			lines++
		}
	}
	if result == text {
		t.Error("expected working context to be trimmed")
	}
}

func TestExtractWorkingContext_ShortText(t *testing.T) {
	text := "Line 1\nLine 2"
	result := extractWorkingContext(text, 5)
	if result != text {
		t.Errorf("expected short text to pass through unchanged, got %q", result)
	}
}

func TestEstimateTokens(t *testing.T) {
	if estimateTokens("") != 0 {
		t.Error("expected 0 tokens for empty string")
	}
	tokens := estimateTokens("hello world")
	if tokens <= 0 {
		t.Errorf("expected positive token count, got %d", tokens)
	}
}

func TestFormatRetrievalResults(t *testing.T) {
	results := []db.SearchResult{
		{Memory: &db.Memory{Content: "Alice prefers dark mode"}, Score: 0.9},
		{Memory: &db.Memory{Content: "Backend uses port 8080"}, Score: 0.7},
	}
	formatted := formatRetrievalResults(results)
	if formatted == "" {
		t.Error("expected non-empty formatted output")
	}
}

func TestAssembler_Construction(t *testing.T) {
	cfg := config.Defaults()
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	a := NewAssembler(database, nil, &cfg.Context)
	if a == nil {
		t.Fatal("expected non-nil assembler")
	}
}

func TestAssembler_Assemble_EmptySession(t *testing.T) {
	cfg := config.Defaults()
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	a := NewAssembler(database, nil, &cfg.Context)
	ctx, err := a.Assemble("test query", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if ctx.UsedTokens < 0 {
		t.Errorf("expected non-negative used tokens, got %d", ctx.UsedTokens)
	}
}

func TestAssembler_Assemble_WithSessionText(t *testing.T) {
	cfg := config.Defaults()
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	a := NewAssembler(database, nil, &cfg.Context)
	sessionText := "User: What is the port?\nAssistant: The backend uses port 8080.\nUser: Thanks!"
	ctx, err := a.Assemble("port number", sessionText, "test-session")
	if err != nil {
		t.Fatal(err)
	}
	hasWorkingCtx := false
	for _, p := range ctx.Pieces {
		if p.Layer == LayerWorkingContext {
			hasWorkingCtx = true
		}
	}
	if !hasWorkingCtx {
		t.Error("expected working context layer to be present")
	}
}

func TestAssembler_TokenBudgetRespected(t *testing.T) {
	cfg := config.Defaults()
	cfg.Context.TokenBudget = 20
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	a := NewAssembler(database, nil, &cfg.Context)
	longText := strings.Repeat("word ", 500)
	ctx, err := a.Assemble("query", longText, "")
	if err != nil {
		t.Fatal(err)
	}
	if ctx.UsedTokens > ctx.Budget+50 {
		t.Errorf("used tokens (%d) exceeds budget (%d) by more than margin", ctx.UsedTokens, ctx.Budget)
	}
}

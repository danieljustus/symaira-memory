package contextassembler

import (
	"testing"
	"time"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
)

func TestAssembler_WorkingMemoryIncluded(t *testing.T) {
	cfg := config.Defaults()
	cfg.WorkingMemory.IncludeInContext = true
	cfg.WorkingMemory.MaxItems = 10

	database, err := db.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	futureExpiry := time.Now().UTC().Add(24 * time.Hour)
	if err := database.SaveMemory(&db.Memory{
		ID:        "wm-1",
		Content:   "Current task: implement feature X",
		Scope:     "project",
		Tier:      "working",
		ExpiresAt: &futureExpiry,
		Metadata:  map[string]string{},
	}); err != nil {
		t.Fatalf("SaveMemory failed: %v", err)
	}

	a := NewAssembler(database, nil, &cfg.Context)
	a.SetWorkingMemoryConfig(&cfg.WorkingMemory)

	ctx, err := a.Assemble("test query", "", "")
	if err != nil {
		t.Fatal(err)
	}

	hasWorkingMem := false
	for _, p := range ctx.Pieces {
		if p.Layer == LayerWorkingMemory {
			hasWorkingMem = true
			break
		}
	}
	if !hasWorkingMem {
		t.Error("expected working memory layer to be present when IncludeInContext=true")
	}
}

func TestAssembler_WorkingMemoryExcluded(t *testing.T) {
	cfg := config.Defaults()
	cfg.WorkingMemory.IncludeInContext = false

	database, err := db.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	futureExpiry := time.Now().UTC().Add(24 * time.Hour)
	if err := database.SaveMemory(&db.Memory{
		ID:        "wm-1",
		Content:   "Current task: implement feature X",
		Scope:     "project",
		Tier:      "working",
		ExpiresAt: &futureExpiry,
		Metadata:  map[string]string{},
	}); err != nil {
		t.Fatalf("SaveMemory failed: %v", err)
	}

	a := NewAssembler(database, nil, &cfg.Context)
	a.SetWorkingMemoryConfig(&cfg.WorkingMemory)

	ctx, err := a.Assemble("test query", "", "")
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range ctx.Pieces {
		if p.Layer == LayerWorkingMemory {
			t.Error("working memory layer should not be present when IncludeInContext=false")
		}
	}
}

func TestAssembler_WorkingMemoryRespectsBudget(t *testing.T) {
	cfg := config.Defaults()
	cfg.Context.TokenBudget = 10
	cfg.WorkingMemory.IncludeInContext = true
	cfg.WorkingMemory.MaxItems = 10

	database, err := db.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	futureExpiry := time.Now().UTC().Add(24 * time.Hour)
	for i := 0; i < 5; i++ {
		if err := database.SaveMemory(&db.Memory{
			ID:        "wm-" + string(rune('a'+i)),
			Content:   "Working memory about databases and networking and security",
			Scope:     "project",
			Tier:      "working",
			ExpiresAt: &futureExpiry,
			Metadata:  map[string]string{},
		}); err != nil {
			t.Fatalf("SaveMemory failed: %v", err)
		}
	}

	a := NewAssembler(database, nil, &cfg.Context)
	a.SetWorkingMemoryConfig(&cfg.WorkingMemory)

	ctx, err := a.Assemble("test", "", "")
	if err != nil {
		t.Fatal(err)
	}

	if ctx.UsedTokens > ctx.Budget+50 {
		t.Errorf("used tokens (%d) exceeds budget (%d) by more than margin", ctx.UsedTokens, ctx.Budget)
	}
}

func TestAssembler_WorkingMemoryRespectsMaxItems(t *testing.T) {
	cfg := config.Defaults()
	cfg.WorkingMemory.IncludeInContext = true
	cfg.WorkingMemory.MaxItems = 2

	database, err := db.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	futureExpiry := time.Now().UTC().Add(24 * time.Hour)
	for i := 0; i < 5; i++ {
		if err := database.SaveMemory(&db.Memory{
			ID:        "wm-item",
			Content:   "Working memory task",
			Scope:     "project",
			Tier:      "working",
			ExpiresAt: &futureExpiry,
			Metadata:  map[string]string{},
		}); err != nil {
			t.Fatalf("SaveMemory failed: %v", err)
		}
	}

	a := NewAssembler(database, nil, &cfg.Context)
	a.SetWorkingMemoryConfig(&cfg.WorkingMemory)

	ctx, err := a.Assemble("test", "", "")
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range ctx.Pieces {
		if p.Layer == LayerWorkingMemory {
			if estimateTokens(p.Content) > cfg.WorkingMemory.MaxItems*20 {
				t.Error("working memory layer content exceeds MaxItems-derived token budget")
			}
		}
	}
}

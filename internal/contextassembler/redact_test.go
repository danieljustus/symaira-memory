package contextassembler

import (
	"strings"
	"testing"
	"time"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
)

// TestAssemble_RedactsLegacySecretInWorkingMemory guards #515 on the
// context-assembly transport: a working memory written without write-time
// redaction (simulated via a direct db.SaveMemory call, bypassing
// internal/memory.Prepare) must not carry raw secret content into the
// assembled prompt context.
func TestAssemble_RedactsLegacySecretInWorkingMemory(t *testing.T) {
	cfg := config.Defaults()
	cfg.WorkingMemory.IncludeInContext = true
	cfg.WorkingMemory.MaxItems = 5

	database, err := db.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()

	secret := "AKIA1234567890ABCDEF"
	futureExpiry := time.Now().UTC().Add(24 * time.Hour)
	if err := database.SaveMemory(&db.Memory{
		ID:        "wm-legacy-secret",
		Content:   "AWS key is " + secret,
		Scope:     "project",
		Tier:      "working",
		ExpiresAt: &futureExpiry,
		Metadata:  map[string]string{},
	}); err != nil {
		t.Fatalf("SaveMemory failed: %v", err)
	}

	a := NewAssembler(database, nil, &cfg.Context)
	a.SetWorkingMemoryConfig(&cfg.WorkingMemory)

	ctx, err := a.Assemble("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ctx.Pieces {
		if strings.Contains(p.Content, secret) {
			t.Fatalf("assembled context leaked raw secret in layer %q: %s", p.Layer, p.Content)
		}
	}
}

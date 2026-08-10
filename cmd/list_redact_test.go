package cmd

import (
	"strings"
	"testing"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
)

// TestListCommandRedactsLegacySecret guards #518 on the CLI list
// transport: a record written without write-time redaction (simulated via
// a direct db.SaveMemory call, bypassing internal/memory.Prepare) must not
// come back with raw secret content from `symmemory list`.
func TestListCommandRedactsLegacySecret(t *testing.T) {
	database := agingTestSetup(t)

	secret := "gho_abcdefabcdefabcdefabcdefabcdefabcdef"
	content := "Deployment note: GitHub auth token is " + secret
	embeddings := extractor.NewEmbeddingsGenerator(config.Defaults())
	emb := embeddings.GenerateVector(content)
	m := &db.Memory{
		ID:              "cli-legacy-secret-list",
		Content:         content,
		Scope:           "global",
		Embedding:       emb.Vector,
		EmbeddingSource: emb.Source,
		EmbeddingModel:  emb.Model,
	}
	if err := database.SaveMemory(m); err != nil {
		t.Fatalf("failed to save test memory: %v", err)
	}

	prevScope, prevEntity, prevAsOf, prevFormat := listScope, listEntity, listAsOf, outputFormat
	listScope, listEntity, listAsOf, outputFormat = "", "", "", "json"
	t.Cleanup(func() { listScope, listEntity, listAsOf, outputFormat = prevScope, prevEntity, prevAsOf, prevFormat })

	out := captureCmdOutput(func() {
		if err := listCmd.RunE(listCmd, nil); err != nil {
			t.Errorf("list command error: %v", err)
		}
	})
	if strings.Contains(out, secret) {
		t.Fatalf("list command output leaked raw secret: %s", out)
	}
	if !strings.Contains(out, "[REDACTED_API_KEY]") {
		t.Fatalf("list command output missing redaction marker; RedactMemories did not run: %s", out)
	}
}

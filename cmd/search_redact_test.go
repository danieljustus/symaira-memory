package cmd

import (
	"strings"
	"testing"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
)

// TestSearchCommandRedactsLegacySecret guards #515 on the CLI search
// transport: a record written without write-time redaction (simulated via
// a direct db.SaveMemory call, bypassing internal/memory.Prepare) must not
// come back with raw secret content from `symmemory search`.
func TestSearchCommandRedactsLegacySecret(t *testing.T) {
	database := agingTestSetup(t)

	secret := "gho_abcdefabcdefabcdefabcdefabcdefabcdef"
	content := "Deployment note: GitHub auth token is " + secret
	embeddings := extractor.NewEmbeddingsGenerator(config.Defaults())
	emb := embeddings.GenerateVector(content)
	m := &db.Memory{
		ID:              "cli-legacy-secret-search",
		Content:         content,
		Scope:           "global",
		Embedding:       emb.Vector,
		EmbeddingSource: emb.Source,
		EmbeddingModel:  emb.Model,
	}
	if err := database.SaveMemory(m); err != nil {
		t.Fatalf("failed to save test memory: %v", err)
	}

	prevScope, prevLimit, prevFormat := searchScope, searchLimit, outputFormat
	searchScope, searchLimit, outputFormat = "", 5, "json"
	t.Cleanup(func() { searchScope, searchLimit, outputFormat = prevScope, prevLimit, prevFormat })

	out := captureCmdOutput(func() {
		if err := searchCmd.RunE(searchCmd, []string{content}); err != nil {
			t.Errorf("search command error: %v", err)
		}
	})
	if strings.Contains(out, secret) {
		t.Fatalf("search command output leaked raw secret: %s", out)
	}
}

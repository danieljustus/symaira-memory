package memory

import (
	"fmt"
	"strings"
	"time"

	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/danieljustus/symaira-memory/internal/security"
	"github.com/google/uuid"
)

// Attribution carries provenance information about who created or updated a
// memory and in which session.
type Attribution struct {
	Author    string // Profile name, "cli:<user>" or JWT Subject
	SessionID string // e.g. Hermes session ID, empty if unknown
}

// Standard provenance metadata keys stored in Memory.Metadata.
// Every memory MUST carry these fields; missing keys imply safe defaults.
//
// Required:
//   - source_type: How the memory was created ("direct", "imported", "extracted")
//   - source_tool: Which tool/client created it ("cli", "mcp", "http", "import:<name>")
//   - source_uri: Origin URI when available (file path, session ID, URL)
//   - observed_at: When the source event occurred (RFC3339)
//
// Optional:
//   - extraction_method: For extracted secondary facts ("pattern", "extractive_summary")
//   - evidence_snippet: Short quote from source that triggered the memory
//   - evidence_hash: SHA-256 of evidence_snippet for dedup
const (
	MetaSourceType       = "source_type"
	MetaSourceTool       = "source_tool"
	MetaSourceURI        = "source_uri"
	MetaObservedAt       = "observed_at"
	MetaExtractionMethod = "extraction_method"
	MetaEvidenceSnippet  = "evidence_snippet"
	MetaEvidenceHash     = "evidence_hash"
)

// Trust metadata keys for retrieval filtering.
// Missing keys imply safe defaults: authority="unverified", confidence="medium",
// verification_status="unverified".
const (
	MetaAuthority           = "authority"
	MetaConfidence          = "confidence"
	MetaVerificationStatus  = "verification_status"
	MetaVerifiedAt          = "verified_at"
	MetaSupersededBy        = "superseded_by"
)

// Trust filter values.
const (
	AuthorityDirect    = "direct"
	AuthorityVerified  = "verified"
	AuthorityInferred  = "inferred"
	AuthorityUnverified = "unverified"

	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"

	VerificationVerified   = "verified"
	VerificationUnverified = "unverified"
	VerificationStale      = "stale"
)

// Sensitivity and sharing policy metadata keys.
// Missing keys imply safe defaults: sensitivity="internal", sharing_level="private".
const (
	MetaSensitivity  = "sensitivity"
	MetaSharingLevel = "sharing_level"
)

// Sensitivity levels (ascending sensitivity).
const (
	SensitivityPublic     = "public"
	SensitivityInternal   = "internal"
	SensitivityConfidential = "confidential"
	SensitivitySecret     = "secret"
)

// Sharing levels (ascending visibility).
const (
	SharingPrivate   = "private"
	SharingTeam      = "team"
	SharingOrg       = "org"
	SharingPublic    = "public"
)

// DefaultProvenance returns the standard provenance metadata for a direct memory
// creation via CLI/MCP/HTTP. The sourceTool parameter identifies the client
// (e.g. "cli", "mcp", "http").
func DefaultProvenance(sourceTool string) map[string]string {
	now := time.Now().UTC().Format(time.RFC3339)
	return map[string]string{
		MetaSourceType: "direct",
		MetaSourceTool: sourceTool,
		MetaSourceURI:  "",
		MetaObservedAt: now,
	}
}

// ImportedProvenance returns provenance metadata for an imported fact.
func ImportedProvenance(sourceTool, sourceURI string, observedAt time.Time) map[string]string {
	ts := observedAt.Format(time.RFC3339)
	return map[string]string{
		MetaSourceType: "imported",
		MetaSourceTool: sourceTool,
		MetaSourceURI:  sourceURI,
		MetaObservedAt: ts,
	}
}

// ExtractionProvenance returns provenance metadata for an extracted secondary fact.
func ExtractionProvenance(method string) map[string]string {
	return map[string]string{
		MetaSourceType:       "extracted",
		MetaExtractionMethod: method,
	}
}

// MergeProvenance overlays provenance onto a base metadata map, preserving
// any pre-existing keys not in the provenance set.
func MergeProvenance(base, provenance map[string]string) map[string]string {
	if base == nil {
		base = make(map[string]string)
	}
	for k, v := range provenance {
		base[k] = v
	}
	return base
}

// Prepare validates scope, redacts PII, detects project boundaries,
// and returns a Memory ready for embedding generation and persistence.
// sourceTool identifies the client (e.g. "cli", "mcp", "http") and is used
// to stamp provenance metadata on the memory.
func Prepare(content, scope string, meta map[string]string, piiEnabled bool, attr Attribution, sourceTool string) (*db.Memory, error) {
	if scope == "" {
		scope = "global"
	}
	if err := security.ValidateScope(scope); err != nil {
		return nil, err
	}

	cleanContent := content
	if piiEnabled {
		cleanContent = security.Redact(content)
	}

	if meta == nil {
		meta = make(map[string]string)
	}

	if scope == "project" {
		detector := security.NewProjectScopeDetector()
		meta["project_name"] = detector.DetectActiveProject()
	}

	provenance := DefaultProvenance(sourceTool)
	meta = MergeProvenance(meta, provenance)

	cleanMeta := meta
	if piiEnabled {
		cleanMeta = security.RedactMap(meta)
	}

	return &db.Memory{
		Content:        cleanContent,
		Scope:          scope,
		Metadata:       cleanMeta,
		CreatedBy:      attr.Author,
		UpdatedBy:      attr.Author,
		CreatedSession: attr.SessionID,
		UpdatedSession: attr.SessionID,
		ValidFrom:      ptrTime(time.Now().UTC()),
	}, nil
}

// Store wraps the full prepare → redact → embed → save → extract-facts pipeline.
// Returns the saved memory and any extracted secondary fact descriptions.
// The entities parameter contains entity names to link to the saved memory.
// sourceTool identifies the client (e.g. "cli", "mcp", "http").
func Store(database *db.DB, embeddings *extractor.EmbeddingsGenerator, patternExtractor *extractor.PatternExtractor, content, scope string, meta map[string]string, piiEnabled bool, attr Attribution, entities []string, sourceTool string) (*db.Memory, []string, error) {
	m, err := Prepare(content, scope, meta, piiEnabled, attr, sourceTool)
	if err != nil {
		return nil, nil, err
	}
	m.ID = uuid.New().String()
	emb := embeddings.GenerateVector(m.Content)
	m.Embedding = emb.Vector
	m.EmbeddingSource = emb.Source
	m.EmbeddingModel = emb.Model

	if err := database.SaveMemory(m); err != nil {
		return nil, nil, fmt.Errorf("failed to save memory: %w", err)
	}

	for _, name := range entities {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		entity, err := database.ResolveEntity(name)
		if err != nil {
			continue
		}
		if entity == nil {
			entity = &db.Entity{
				ID:        uuid.New().String(),
				Name:      name,
				Type:      "other",
				Aliases:   []string{},
				CreatedBy: attr.Author,
				CreatedAt: time.Now().UTC(),
			}
			if err := database.SaveEntity(entity); err != nil {
				continue
			}
		}
		_ = database.LinkMemoryToEntity(m.ID, entity.ID)
		m.Entities = append(m.Entities, entity.Name)
	}

	extractedFacts := patternExtractor.ExtractFacts(m.Content)
	var extractedStr []string
	for _, f := range extractedFacts {
		cleanFactContent := f.Content
		if piiEnabled {
			cleanFactContent = security.Redact(f.Content)
		}

		if isDuplicateOfPrimary(cleanFactContent, f.Metadata["raw_trigger"], m.Content) {
			continue
		}

		subID := uuid.New().String()
		subEmb := embeddings.GenerateVector(cleanFactContent)
		subVector := subEmb.Vector

		subMeta := f.Metadata
		if subMeta == nil {
			subMeta = make(map[string]string)
		}
		if m.Scope == "project" {
			subMeta["project_name"] = m.Metadata["project_name"]
		}

		extractionProv := ExtractionProvenance("pattern")
		subMeta = MergeProvenance(subMeta, extractionProv)

		if piiEnabled {
			subMeta = security.RedactMap(subMeta)
		}

		subMem := &db.Memory{
			ID:              subID,
			Content:         cleanFactContent,
			Scope:           m.Scope,
			Metadata:        subMeta,
			Embedding:       subVector,
			EmbeddingSource: subEmb.Source,
			EmbeddingModel:  subEmb.Model,
			CreatedBy:       attr.Author,
			UpdatedBy:       attr.Author,
			CreatedSession:  attr.SessionID,
			UpdatedSession:  attr.SessionID,
		}
		if err := database.SaveMemory(subMem); err == nil {
			extractedStr = append(extractedStr, fmt.Sprintf("  - [Fact Extracted] %s (ID: %s)", cleanFactContent, subID))
		}
	}

	return m, extractedStr, nil
}

// FormatStoreSuccess builds a human-readable success message for a stored memory.
func FormatStoreSuccess(m *db.Memory, extractedStr []string) string {
	responseMsg := fmt.Sprintf("Successfully saved memory!\nMemory ID: %s\nContent: %s\nScope: %s", m.ID, m.Content, m.Scope)
	if m.Scope == "project" {
		responseMsg += fmt.Sprintf("\nProject: %s", m.Metadata["project_name"])
	}
	if len(extractedStr) > 0 {
		responseMsg += "\n\nAdditionally, secondary facts were successfully extracted:\n" + strings.Join(extractedStr, "\n")
	}
	return responseMsg
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func isDuplicateOfPrimary(fact, rawTrigger, primary string) bool {
	norm := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		return strings.Trim(s, `.*"';!?`)
	}
	factNorm := norm(fact)
	primaryNorm := norm(primary)
	if factNorm == "" {
		return false
	}
	if factNorm == primaryNorm || strings.Contains(primaryNorm, factNorm) {
		return true
	}
	if rawTrigger != "" {
		return norm(rawTrigger) == primaryNorm
	}
	return false
}

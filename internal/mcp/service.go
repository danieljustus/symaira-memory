package mcp

import (
	"fmt"
	"time"

	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/danieljustus/symaira-memory/internal/memory"
	"github.com/danieljustus/symaira-memory/internal/security"
)

// MemoryService encapsulates all business logic for memory operations,
// decoupling HTTP handlers from database and extraction internals.
type MemoryService struct {
	db         *db.DB
	extractor  *extractor.PatternExtractor
	embeddings *extractor.EmbeddingsGenerator
	piiEnabled bool
}

// NewMemoryService creates a service with the given dependencies.
func NewMemoryService(database *db.DB, embeddings *extractor.EmbeddingsGenerator, piiEnabled bool) *MemoryService {
	return &MemoryService{
		db:         database,
		extractor:  extractor.NewPatternExtractor(),
		embeddings: embeddings,
		piiEnabled: piiEnabled,
	}
}

func (s *MemoryService) SetPIIEnabled(enabled bool) {
	s.piiEnabled = enabled
}

func (s *MemoryService) ActiveBackend() string {
	return s.embeddings.ActiveBackend()
}

func (s *MemoryService) Search(query, scope string, limit int, entityName string, trustFilter db.TrustFilter, policyFilter db.PolicyFilter) ([]db.SearchResult, error) {
	var entityID string
	if entityName != "" {
		entity, err := s.db.ResolveEntity(entityName)
		if err != nil {
			return nil, fmt.Errorf("resolve entity: %w", err)
		}
		if entity == nil {
			return nil, &NotFoundError{Resource: "entity", Identifier: entityName}
		}
		entityID = entity.ID
	}

	emb := s.embeddings.GenerateVector(query)
	return s.db.SearchMemoriesFilteredWithTrust(emb.Vector, emb.Source, scope, limit, entityID, trustFilter, policyFilter)
}

func (s *MemoryService) SearchWithProfile(query, profileName string, limit int, entityName string, trustFilter db.TrustFilter, policyFilter db.PolicyFilter) ([]db.SearchResult, error) {
	var entityID string
	if entityName != "" {
		entity, err := s.db.ResolveEntity(entityName)
		if err != nil {
			return nil, fmt.Errorf("resolve entity: %w", err)
		}
		if entity == nil {
			return nil, &NotFoundError{Resource: "entity", Identifier: entityName}
		}
		entityID = entity.ID
	}

	emb := s.embeddings.GenerateVector(query)
	return s.db.SearchMemoriesWithProfile(emb.Vector, emb.Source, profileName, limit, entityID, trustFilter, policyFilter)
}

func (s *MemoryService) Set(content, scope string, metadata map[string]string, sessionID string, author string, entities []string, sourceTool string) (string, error) {
	attr := memory.Attribution{
		Author:    author,
		SessionID: sessionID,
	}
	m, _, err := memory.Store(s.db, s.embeddings, s.extractor, content, scope, metadata, s.piiEnabled, attr, entities, sourceTool)
	if err != nil {
		return "", err
	}
	return m.ID, nil
}

func (s *MemoryService) Get(id string) (*db.Memory, error) {
	m, err := s.db.GetMemory(id)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, &NotFoundError{Resource: "memory", Identifier: id}
	}
	return m, nil
}

func (s *MemoryService) Delete(id string) error {
	m, err := s.db.GetMemory(id)
	if err != nil {
		return err
	}
	if m == nil {
		return &NotFoundError{Resource: "memory", Identifier: id}
	}
	return s.db.DeleteMemory(id)
}

func (s *MemoryService) List(scope string, limit int) ([]*db.Memory, error) {
	return s.db.ListMemoriesLite(scope, 0, limit)
}

func (s *MemoryService) ListWithPolicy(scope string, limit int, policyFilter db.PolicyFilter) ([]*db.Memory, error) {
	if policyFilter.MaxSensitivity == "" && policyFilter.MinSharingLevel == "" && policyFilter.ClientID == "" {
		return s.db.ListMemoriesLite(scope, 0, limit)
	}
	return s.db.ListMemoriesFilteredWithPolicy(scope, 0, limit, policyFilter)
}

func (s *MemoryService) ListRules(scope string) ([]*db.Rule, error) {
	return s.db.ListRules(scope)
}

func (s *MemoryService) ListEntities() ([]*db.Entity, error) {
	return s.db.ListEntities()
}

func (s *MemoryService) ResolveEntity(nameOrAlias string) (*db.Entity, error) {
	return s.db.ResolveEntity(nameOrAlias)
}

func (s *MemoryService) ResolveEntityCandidates(query, entityType string, aliasHints []string, limit int) ([]db.EntityCandidate, error) {
	return s.db.ResolveEntityCandidates(query, entityType, aliasHints, limit)
}

func (s *MemoryService) GetEntityByID(id string) (*db.Entity, error) {
	return s.db.GetEntityByID(id)
}

func (s *MemoryService) SaveEntityRelationProvenance(r *db.EntityRelation) (*db.EntityRelation, error) {
	return s.db.SaveEntityRelationProvenance(r)
}

func (s *MemoryService) SaveEntityRelation(r *db.EntityRelation) error {
	return s.db.SaveEntityRelation(r)
}

func (s *MemoryService) DeleteEntityRelation(fromEntityID, toEntityID, relationType string) error {
	return s.db.DeleteEntityRelation(fromEntityID, toEntityID, relationType)
}

func (s *MemoryService) GraphNeighbors(entityID string, depth int) ([]*db.Entity, []*db.EntityRelation, error) {
	return s.db.GraphNeighbors(entityID, depth)
}

func (s *MemoryService) ListMemoriesAsOf(scope string, asOf time.Time, limit int) ([]*db.Memory, error) {
	return s.db.ListMemoriesAsOf(scope, asOf, 0, limit)
}

func (s *MemoryService) GetMemory(id string) (*db.Memory, error) {
	return s.db.GetMemory(id)
}

func (s *MemoryService) GetMemoryEvidence(memoryID string) ([]db.EvidenceSpan, error) {
	return s.db.GetMemoryEvidence(memoryID)
}

func (s *MemoryService) GetMemoriesSinceCursor(since time.Time, limit int, includeEmbeddings ...bool) ([]*db.Memory, error) {
	return s.db.GetMemoriesSinceCursor(since, limit, includeEmbeddings...)
}

func (s *MemoryService) UpsertMemoryIfNewer(m *db.Memory) (bool, error) {
	if s.piiEnabled {
		m.Content = security.Redact(m.Content)
		m.Metadata = security.RedactMap(m.Metadata)
	}
	return s.db.UpsertMemoryIfNewer(m)
}

// SyncUpsertMemoryIfNewer is the tombstone-aware upsert used by sync
// ingestion; it never resurrects a memory deleted after the incoming row.
func (s *MemoryService) SyncUpsertMemoryIfNewer(m *db.Memory) (bool, error) {
	if s.piiEnabled {
		m.Content = security.Redact(m.Content)
		m.Metadata = security.RedactMap(m.Metadata)
	}
	return s.db.SyncUpsertMemoryIfNewer(m)
}

func (s *MemoryService) GetDeletedSince(since time.Time) ([]db.DeletedMemory, error) {
	return s.db.GetDeletedSince(since)
}

func (s *MemoryService) ApplyRemoteDelete(id string, deletedAt time.Time) (bool, error) {
	return s.db.ApplyRemoteDelete(id, deletedAt)
}

func (s *MemoryService) StoreRelayBlob(b db.RelayBlob) (bool, error) {
	return s.db.StoreRelayBlob(b)
}

func (s *MemoryService) GetRelayBlobsSince(since time.Time, limit int) ([]db.RelayBlob, error) {
	return s.db.GetRelayBlobsSince(since, limit)
}

func (s *MemoryService) LogAudit(action, entityID, memoryID, diff, actor, detail string) error {
	return s.db.LogAudit(action, entityID, memoryID, diff, actor, detail)
}

// NotFoundError indicates a requested resource does not exist.
type NotFoundError struct {
	Resource   string
	Identifier string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s not found: %s", e.Resource, e.Identifier)
}

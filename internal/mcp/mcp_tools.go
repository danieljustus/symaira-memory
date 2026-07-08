package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/danieljustus/symaira-corekit/mcpserver"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/instructions"
	"github.com/danieljustus/symaira-memory/internal/security"
)

type MemoryResponse struct {
	ID                  string            `json:"id"`
	Content             string            `json:"content"`
	Scope               string            `json:"scope"`
	Metadata            map[string]string `json:"metadata,omitempty"`
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
	CreatedBy           string            `json:"created_by,omitempty"`
	CreatedSession      string            `json:"created_session,omitempty"`
	Entities            []string          `json:"entities,omitempty"`
	ConsolidationStatus string            `json:"consolidation_status,omitempty"`
	EmbeddingSource     string            `json:"embedding_source,omitempty"`
	EmbeddingModel      string            `json:"embedding_model,omitempty"`
	Importance          float64           `json:"importance,omitempty"`
	Evidence            []db.EvidenceSpan `json:"evidence,omitempty"`
}

type SearchResultResponse struct {
	Memory MemoryResponse `json:"memory"`
	Score  float32        `json:"score"`
}

func memoryResponse(m *db.Memory) MemoryResponse {
	if m == nil {
		return MemoryResponse{}
	}
	return MemoryResponse{
		ID:                  m.ID,
		Content:             m.Content,
		Scope:               m.Scope,
		Metadata:            m.Metadata,
		CreatedAt:           m.CreatedAt,
		UpdatedAt:           m.UpdatedAt,
		CreatedBy:           m.CreatedBy,
		CreatedSession:      m.CreatedSession,
		Entities:            m.Entities,
		ConsolidationStatus: m.ConsolidationStatus,
		EmbeddingSource:     m.EmbeddingSource,
		EmbeddingModel:      m.EmbeddingModel,
		Importance:          m.Importance,
	}
}

func searchResultResponse(r db.SearchResult) SearchResultResponse {
	return SearchResultResponse{
		Memory: memoryResponse(r.Memory),
		Score:  r.Score,
	}
}

func (s *Server) MCPServer() *mcpserver.Server {
	srv := mcpserver.New("symaira-memory", s.version)
	srv.SetInstructions(instructions.Text(s.version))

	srv.RegisterTool(&mcpserver.Tool{
		Name:        "memory_get",
		Description: "Retrieve a specific memory by its unique ID.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"Unique memory UUID"},"client_id":{"type":"string","description":"Optional client ID for access control filtering"},"with_evidence":{"type":"boolean","description":"Optional: include grounded evidence spans backing this memory, if any (default false)"}},"required":["id"]}`),
		Handler:     s.handleMemoryGet,
	})

	srv.RegisterTool(&mcpserver.Tool{
		Name:        "memory_set",
		Description: "Save a new persistent memory or fact. Use this tool autonomously when the user expresses a clear preference, constraint, architectural decision, or guideline that should persist across sessions.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"content":{"type":"string","description":"The text content or fact to remember (e.g., 'User prefers TypeScript for script tasks' or 'API uses port 8080'). Keep it concise and objective."},"scope":{"type":"string","description":"Scope level: 'global' (default, for general user settings), 'project' (highly recommended for folder-specific codebases; auto-resolves project name using .symmemory.toml or .git in CWD), 'agent', 'user', or 'session'"},"metadata":{"type":"string","description":"Optional JSON metadata key-value string (e.g., '{\"source\": \"claude-agent\"}')"},"session_id":{"type":"string","description":"Optional session ID for provenance tracking (e.g., the current chat/conversation session identifier)"},"entities":{"type":"string","description":"Optional comma-separated entity names to link (e.g., 'Irene,Premium BnB'). Entities are auto-created if they don't exist."}},"required":["content"]}`),
		Handler:     s.handleMemorySet,
	})

	srv.RegisterTool(&mcpserver.Tool{
		Name:        "memory_search",
		Description: "Perform a semantic vector similarity search on stored memories. Always use this tool at the start of a session or task to retrieve relevant past design decisions, user preferences, and project guidelines.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"The natural language query or semantic term (e.g., 'database port' or 'language preference')"},"scope":{"type":"string","description":"Optional scope level filter ('global', 'project', 'agent', 'user', 'session')"},"limit":{"type":"integer","description":"Optional maximum number of search results to return (default 5)"},"entity":{"type":"string","description":"Optional entity name filter — only returns memories linked to this entity"},"min_confidence":{"type":"string","description":"Optional minimum confidence level filter ('low', 'medium', 'high')"},"verification":{"type":"string","description":"Optional verification status filter ('verified', 'unverified', 'stale')"},"exclude_superseded":{"type":"boolean","description":"Optional exclude memories that have been superseded (default false)"},"max_age":{"type":"string","description":"Optional maximum memory age (e.g. '7d', '30d', '1y')"},"max_sensitivity":{"type":"string","description":"Optional maximum sensitivity level ('public', 'internal', 'confidential', 'secret')"},"min_sharing_level":{"type":"string","description":"Optional minimum sharing level ('private', 'team', 'org', 'public')"},"client_id":{"type":"string","description":"Optional client ID for access control filtering"},"with_evidence":{"type":"boolean","description":"Optional: include grounded evidence spans for each result, if any (default false)"}},"required":["query"]}`),
		Handler:     s.handleMemorySearch,
	})

	srv.RegisterTool(&mcpserver.Tool{
		Name:        "memory_list",
		Description: "List all memories currently stored in the database. Useful for debugging or displaying stored context lists.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"scope":{"type":"string","description":"Optional scope level filter ('global', 'project', 'agent', 'user', 'session')"},"limit":{"type":"integer","description":"Optional maximum number of memories to return (default 100, max 1000)"},"max_sensitivity":{"type":"string","description":"Optional maximum sensitivity level ('public', 'internal', 'confidential', 'secret')"},"min_sharing_level":{"type":"string","description":"Optional minimum sharing level ('private', 'team', 'org', 'public')"},"client_id":{"type":"string","description":"Optional client ID for access control filtering"},"as_of":{"type":"string","description":"Optional RFC3339 timestamp: return memory state as of this point in time instead of current state. Not combinable with the policy filters."}}}`),
		Handler:     s.handleMemoryList,
	})

	srv.RegisterTool(&mcpserver.Tool{
		Name:        "entity_list",
		Description: "List all known entities (people, projects, organizations). Use this to discover which entities exist before linking memories or filtering searches.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		Handler:     s.handleEntityList,
	})

	srv.RegisterTool(&mcpserver.Tool{
		Name:        "entity_relate",
		Description: "Create or delete a directed, typed relationship between two entities (e.g. 'Alice works-with Bob'). Use action='delete' to remove a relation.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"from":{"type":"string","description":"Name or alias of the source entity"},"relation":{"type":"string","description":"Relation type, free-form (e.g. 'works-with', 'manages', 'depends-on')"},"to":{"type":"string","description":"Name or alias of the target entity"},"action":{"type":"string","description":"'create' (default) or 'delete'"}},"required":["from","relation","to"]}`),
		Handler:     s.handleEntityRelate,
	})

	srv.RegisterTool(&mcpserver.Tool{
		Name:        "graph_neighbors",
		Description: "Return the entities and relations reachable from a starting entity via a breadth-first traversal, as {nodes, edges}. Use this to answer 'what connects to X'.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"entity":{"type":"string","description":"Name or alias of the starting entity"},"depth":{"type":"integer","description":"Traversal depth, 1-3 (default 1)"}},"required":["entity"]}`),
		Handler:     s.handleGraphNeighbors,
	})

	return srv
}

func (s *Server) handleMemoryGet(ctx context.Context, input json.RawMessage) (any, error) {
	var args struct {
		ID           string `json:"id"`
		ClientID     string `json:"client_id"`
		WithEvidence bool   `json:"with_evidence"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments for 'memory_get': failed to parse arguments: %w", err)
	}
	if args.ID == "" {
		return nil, fmt.Errorf("invalid arguments for 'memory_get': 'id' is required")
	}

	m, err := s.service.Get(args.ID)
	if err != nil {
		if nf, ok := err.(*NotFoundError); ok {
			return nf.Error(), nil
		}
		return mcpError("Failed to fetch memory", err)
	}

	if args.ClientID != "" {
		policyFilter := db.PolicyFilter{ClientID: args.ClientID}
		if !db.PassesPolicyFilter(m, policyFilter) {
			return nil, fmt.Errorf("access denied: memory %s is not accessible by client %s", args.ID, args.ClientID)
		}
	}

	resp := memoryResponse(m)
	if args.WithEvidence {
		evidence, err := s.service.GetMemoryEvidence(args.ID)
		if err != nil {
			return mcpError("Failed to fetch memory evidence", err)
		}
		resp.Evidence = evidence
	}

	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *Server) handleMemorySet(ctx context.Context, input json.RawMessage) (any, error) {
	if s.profile != nil && !security.ParseRole(s.profile.Role).CanWrite() {
		return nil, fmt.Errorf("permission denied: profile role is read-only")
	}

	var args struct {
		Content   string `json:"content"`
		Scope     string `json:"scope"`
		Metadata  string `json:"metadata"`
		SessionID string `json:"session_id"`
		Entities  string `json:"entities"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments for 'memory_set': failed to parse arguments: %w", err)
	}
	if args.Content == "" {
		return nil, fmt.Errorf("invalid arguments for 'memory_set': 'content' is required")
	}

	meta := make(map[string]string)
	if args.Metadata != "" {
		if err := json.Unmarshal([]byte(args.Metadata), &meta); err != nil {
			return nil, fmt.Errorf("invalid arguments for 'memory_set': 'metadata' must be a valid JSON object: %w", err)
		}
	}

	var entityNames []string
	if args.Entities != "" {
		for _, e := range strings.Split(args.Entities, ",") {
			e = strings.TrimSpace(e)
			if e != "" {
				entityNames = append(entityNames, e)
			}
		}
	}

	id, err := s.service.Set(args.Content, args.Scope, meta, args.SessionID, "mcp", entityNames, "mcp")
	if err != nil {
		return nil, fmt.Errorf("%s", err.Error())
	}

	return fmt.Sprintf("Memory saved successfully with ID: %s", id), nil
}

func (s *Server) handleMemorySearch(ctx context.Context, input json.RawMessage) (any, error) {
	var args struct {
		Query             string `json:"query"`
		Scope             string `json:"scope"`
		Limit             int    `json:"limit"`
		Entity            string `json:"entity"`
		MinConfidence     string `json:"min_confidence"`
		Verification      string `json:"verification"`
		ExcludeSuperseded bool   `json:"exclude_superseded"`
		MaxAge            string `json:"max_age"`
		MaxSensitivity    string `json:"max_sensitivity"`
		MinSharingLevel   string `json:"min_sharing_level"`
		ClientID          string `json:"client_id"`
		WithEvidence      bool   `json:"with_evidence"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments for 'memory_search': failed to parse arguments: %w", err)
	}
	if args.Query == "" {
		return nil, fmt.Errorf("invalid arguments for 'memory_search': 'query' is required")
	}

	limit := args.Limit
	if limit <= 0 {
		limit = 5
	}

	trustFilter := db.TrustFilter{
		MinConfidence:      args.MinConfidence,
		VerificationStatus: args.Verification,
		ExcludeSuperseded:  args.ExcludeSuperseded,
	}
	if args.MaxAge != "" {
		dur, err := parseDuration(args.MaxAge)
		if err != nil {
			return nil, fmt.Errorf("invalid arguments for 'memory_search': invalid max_age: %w", err)
		}
		trustFilter.MaxAge = dur
	}

	policyFilter := db.PolicyFilter{
		MaxSensitivity:  args.MaxSensitivity,
		MinSharingLevel: args.MinSharingLevel,
		ClientID:        args.ClientID,
	}

	results, err := s.service.Search(args.Query, args.Scope, limit, args.Entity, trustFilter, policyFilter)
	if err != nil {
		if nf, ok := err.(*NotFoundError); ok {
			return nil, fmt.Errorf("%s", nf.Error())
		}
		return mcpError("Failed to search memories", err)
	}

	if len(results) == 0 {
		return "No relevant memories found.", nil
	}

	compact := make([]SearchResultResponse, len(results))
	for i, r := range results {
		compact[i] = searchResultResponse(r)
		if args.WithEvidence && r.Memory != nil {
			evidence, err := s.service.GetMemoryEvidence(r.Memory.ID)
			if err == nil {
				compact[i].Memory.Evidence = evidence
			}
		}
	}

	data, _ := json.MarshalIndent(compact, "", "  ")
	return string(data), nil
}

func (s *Server) handleMemoryList(ctx context.Context, input json.RawMessage) (any, error) {
	var args struct {
		Scope           string `json:"scope"`
		Limit           int    `json:"limit"`
		MaxSensitivity  string `json:"max_sensitivity"`
		MinSharingLevel string `json:"min_sharing_level"`
		ClientID        string `json:"client_id"`
		AsOf            string `json:"as_of"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments for 'memory_list': failed to parse arguments: %w", err)
	}

	limit := args.Limit
	if limit < 1 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	if args.AsOf != "" {
		asOf, err := time.Parse(time.RFC3339, args.AsOf)
		if err != nil {
			return nil, fmt.Errorf("invalid arguments for 'memory_list': 'as_of' must be RFC3339: %w", err)
		}
		memories, err := s.service.ListMemoriesAsOf(args.Scope, asOf, limit)
		if err != nil {
			return mcpError("Failed to list memories as of the given time", err)
		}
		if len(memories) == 0 {
			return "No memories were valid at that point in time.", nil
		}
		data, _ := json.MarshalIndent(memories, "", "  ")
		return string(data), nil
	}

	policyFilter := db.PolicyFilter{
		MaxSensitivity:  args.MaxSensitivity,
		MinSharingLevel: args.MinSharingLevel,
		ClientID:        args.ClientID,
	}

	memories, err := s.service.ListWithPolicy(args.Scope, limit, policyFilter)
	if err != nil {
		return mcpError("Failed to list memories", err)
	}

	if len(memories) == 0 {
		return "Memory store is empty.", nil
	}

	data, _ := json.MarshalIndent(memories, "", "  ")
	return string(data), nil
}

func (s *Server) handleEntityList(ctx context.Context, input json.RawMessage) (any, error) {
	entities, err := s.service.ListEntities()
	if err != nil {
		return mcpError("Failed to list entities", err)
	}

	if len(entities) == 0 {
		return "No entities found.", nil
	}

	data, _ := json.MarshalIndent(entities, "", "  ")
	return string(data), nil
}

func (s *Server) handleEntityRelate(ctx context.Context, input json.RawMessage) (any, error) {
	var args struct {
		From     string `json:"from"`
		Relation string `json:"relation"`
		To       string `json:"to"`
		Action   string `json:"action"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments for 'entity_relate': failed to parse arguments: %w", err)
	}
	if args.From == "" || args.Relation == "" || args.To == "" {
		return nil, fmt.Errorf("invalid arguments for 'entity_relate': 'from', 'relation', and 'to' are required")
	}
	if args.Action == "" {
		args.Action = "create"
	}

	fromEntity, err := s.service.ResolveEntity(args.From)
	if err != nil {
		return mcpError("Failed to resolve source entity", err)
	}
	if fromEntity == nil {
		return nil, fmt.Errorf("entity not found: %s", args.From)
	}
	toEntity, err := s.service.ResolveEntity(args.To)
	if err != nil {
		return mcpError("Failed to resolve target entity", err)
	}
	if toEntity == nil {
		return nil, fmt.Errorf("entity not found: %s", args.To)
	}

	switch args.Action {
	case "create":
		rel := &db.EntityRelation{
			FromEntityID: fromEntity.ID,
			ToEntityID:   toEntity.ID,
			RelationType: args.Relation,
			CreatedBy:    "mcp",
			CreatedAt:    time.Now().UTC(),
		}
		if err := s.service.SaveEntityRelation(rel); err != nil {
			return mcpError("Failed to save relation", err)
		}
		return fmt.Sprintf("Related: %s --%s--> %s", fromEntity.Name, args.Relation, toEntity.Name), nil
	case "delete":
		if err := s.service.DeleteEntityRelation(fromEntity.ID, toEntity.ID, args.Relation); err != nil {
			return mcpError("Failed to delete relation", err)
		}
		return fmt.Sprintf("Unrelated: %s --%s--> %s", fromEntity.Name, args.Relation, toEntity.Name), nil
	default:
		return nil, fmt.Errorf("invalid arguments for 'entity_relate': 'action' must be 'create' or 'delete', got %q", args.Action)
	}
}

func (s *Server) handleGraphNeighbors(ctx context.Context, input json.RawMessage) (any, error) {
	var args struct {
		Entity string `json:"entity"`
		Depth  int    `json:"depth"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments for 'graph_neighbors': failed to parse arguments: %w", err)
	}
	if args.Entity == "" {
		return nil, fmt.Errorf("invalid arguments for 'graph_neighbors': 'entity' is required")
	}
	if args.Depth == 0 {
		args.Depth = 1
	}

	entity, err := s.service.ResolveEntity(args.Entity)
	if err != nil {
		return mcpError("Failed to resolve entity", err)
	}
	if entity == nil {
		return nil, fmt.Errorf("entity not found: %s", args.Entity)
	}

	nodes, edges, err := s.service.GraphNeighbors(entity.ID, args.Depth)
	if err != nil {
		return nil, fmt.Errorf("invalid arguments for 'graph_neighbors': %w", err)
	}

	data, _ := json.MarshalIndent(struct {
		Nodes []*db.Entity         `json:"nodes"`
		Edges []*db.EntityRelation `json:"edges"`
	}{Nodes: nodes, Edges: edges}, "", "  ")
	return string(data), nil
}

func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	if len(s) < 2 {
		return 0, fmt.Errorf("invalid duration: %s", s)
	}
	suffix := s[len(s)-1]
	switch suffix {
	case 'd':
		n, err := strconv.Atoi(s[:len(s)-1])
		if err != nil {
			return 0, err
		}
		return time.Duration(n) * 24 * time.Hour, nil
	case 'h':
		return time.ParseDuration(s)
	case 'm':
		return time.ParseDuration(s)
	case 's':
		return time.ParseDuration(s)
	default:
		return time.ParseDuration(s)
	}
}

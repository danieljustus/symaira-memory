package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/danieljustus/symaira-corekit/mcpserver"
	"github.com/danieljustus/symaira-memory/internal/memory"
	"github.com/danieljustus/symaira-memory/internal/security"
)

func (s *Server) MCPServer() *mcpserver.Server {
	srv := mcpserver.New("symaira-memory", s.version)

	srv.RegisterTool(&mcpserver.Tool{
		Name:        "memory_get",
		Description: "Retrieve a specific memory by its unique ID.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"Unique memory UUID"}},"required":["id"]}`),
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
		InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"The natural language query or semantic term (e.g., 'database port' or 'language preference')"},"scope":{"type":"string","description":"Optional scope level filter ('global', 'project', 'agent', 'user', 'session')"},"limit":{"type":"integer","description":"Optional maximum number of search results to return (default 5)"},"entity":{"type":"string","description":"Optional entity name filter — only returns memories linked to this entity"}},"required":["query"]}`),
		Handler:     s.handleMemorySearch,
	})

	srv.RegisterTool(&mcpserver.Tool{
		Name:        "memory_list",
		Description: "List all memories currently stored in the database. Useful for debugging or displaying stored context lists.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"scope":{"type":"string","description":"Optional scope level filter ('global', 'project', 'agent', 'user', 'session')"},"limit":{"type":"integer","description":"Optional maximum number of memories to return (default 100, max 1000)"}}}`),
		Handler:     s.handleMemoryList,
	})

	srv.RegisterTool(&mcpserver.Tool{
		Name:        "entity_list",
		Description: "List all known entities (people, projects, organizations). Use this to discover which entities exist before linking memories or filtering searches.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		Handler:     s.handleEntityList,
	})

	return srv
}

func (s *Server) handleMemoryGet(ctx context.Context, input json.RawMessage) (any, error) {
	var args struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("Invalid arguments for 'memory_get': failed to parse arguments: %w", err)
	}
	if args.ID == "" {
		return nil, fmt.Errorf("Invalid arguments for 'memory_get': 'id' is required")
	}

	m, err := s.db.GetMemory(args.ID)
	if err != nil {
		return mcpError("Failed to fetch memory", err)
	}

	if m == nil {
		return "Memory not found", nil
	}

	data, _ := json.MarshalIndent(m, "", "  ")
	return string(data), nil
}

func (s *Server) handleMemorySet(ctx context.Context, input json.RawMessage) (any, error) {
	if s.profile != nil && !security.ParseRole(s.profile.Role).CanWrite() {
		return nil, fmt.Errorf("Permission denied: profile role is read-only")
	}

	var args struct {
		Content   string `json:"content"`
		Scope     string `json:"scope"`
		Metadata  string `json:"metadata"`
		SessionID string `json:"session_id"`
		Entities  string `json:"entities"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("Invalid arguments for 'memory_set': failed to parse arguments: %w", err)
	}
	if args.Content == "" {
		return nil, fmt.Errorf("Invalid arguments for 'memory_set': 'content' is required")
	}

	meta := make(map[string]string)
	if args.Metadata != "" {
		_ = json.Unmarshal([]byte(args.Metadata), &meta)
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

	attr := memory.Attribution{
		Author:    "mcp",
		SessionID: args.SessionID,
	}

	m, extractedStr, err := memory.Store(s.db, s.embeddings, s.extractor, args.Content, args.Scope, meta, s.piiEnabled, attr, entityNames)
	if err != nil {
		return nil, fmt.Errorf("%s", err.Error())
	}

	return memory.FormatStoreSuccess(m, extractedStr), nil
}

func (s *Server) handleMemorySearch(ctx context.Context, input json.RawMessage) (any, error) {
	var args struct {
		Query  string `json:"query"`
		Scope  string `json:"scope"`
		Limit  int    `json:"limit"`
		Entity string `json:"entity"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("Invalid arguments for 'memory_search': failed to parse arguments: %w", err)
	}
	if args.Query == "" {
		return nil, fmt.Errorf("Invalid arguments for 'memory_search': 'query' is required")
	}

	limit := args.Limit
	if limit <= 0 {
		limit = 5
	}

	var entityID string
	if args.Entity != "" {
		entity, err := s.db.ResolveEntity(args.Entity)
		if err != nil {
			return mcpError("Failed to resolve entity", err)
		}
		if entity == nil {
			return nil, fmt.Errorf("Entity not found: %s", args.Entity)
		}
		entityID = entity.ID
	}

	queryVector := s.embeddings.GenerateVector(args.Query)
	results, err := s.db.SearchMemoriesFiltered(queryVector, args.Scope, limit, entityID)
	if err != nil {
		return mcpError("Failed to search memories", err)
	}

	if len(results) == 0 {
		return "No relevant memories found.", nil
	}

	data, _ := json.MarshalIndent(results, "", "  ")
	return string(data), nil
}

func (s *Server) handleMemoryList(ctx context.Context, input json.RawMessage) (any, error) {
	var args struct {
		Scope string `json:"scope"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("Invalid arguments for 'memory_list': failed to parse arguments: %w", err)
	}

	limit := args.Limit
	if limit < 1 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	memories, err := s.db.ListMemoriesLite(args.Scope, 0, limit)
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
	entities, err := s.db.ListEntities()
	if err != nil {
		return mcpError("Failed to list entities", err)
	}

	if len(entities) == 0 {
		return "No entities found.", nil
	}

	data, _ := json.MarshalIndent(entities, "", "  ")
	return string(data), nil
}

package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/danieljustus/symaira-corekit/mcpserver"
	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/danieljustus/symaira-memory/internal/memory"
	"github.com/danieljustus/symaira-memory/internal/security"
	"github.com/danieljustus/symaira-memory/internal/web"
	"github.com/google/uuid"
)

type Server struct {
	db             *db.DB
	extractor      *extractor.PatternExtractor
	embeddings     *extractor.EmbeddingsGenerator
	jwts           *security.JWTProvider
	allowedOrigins []string
	piiEnabled     bool
	version        string
	cfg            *config.Config
	profile        *db.Profile
	rateLimiter    *RateLimiter
}

func NewServer(database *db.DB, jwtProvider *security.JWTProvider, version string, cfg *config.Config) *Server {
	return &Server{
		db:             database,
		extractor:      extractor.NewPatternExtractor(),
		embeddings:     extractor.NewEmbeddingsGenerator(cfg),
		jwts:           jwtProvider,
		allowedOrigins: []string{"chrome-extension://*", "moz-extension://*"},
		piiEnabled:     true,
		version:        version,
		cfg:            cfg,
		rateLimiter:    NewRateLimiter(DefaultRateLimitConfig()),
	}
}

func (s *Server) SetPIIEnabled(enabled bool) {
	s.piiEnabled = enabled
}

func (s *Server) SetAllowedOrigins(origins []string) {
	s.allowedOrigins = origins
}

func (s *Server) SetProfile(p *db.Profile) {
	s.profile = p
}

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
		fmt.Fprintf(os.Stderr, "[MCP ERROR] Failed to fetch memory: %v\n", err)
		return nil, fmt.Errorf("Failed to fetch memory")
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
			fmt.Fprintf(os.Stderr, "[MCP ERROR] Failed to resolve entity: %v\n", err)
			return nil, fmt.Errorf("Failed to resolve entity")
		}
		if entity == nil {
			return nil, fmt.Errorf("Entity not found: %s", args.Entity)
		}
		entityID = entity.ID
	}

	queryVector := s.embeddings.GenerateVector(args.Query)
	results, err := s.db.SearchMemoriesFiltered(queryVector, args.Scope, limit, entityID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[MCP ERROR] Failed to search memories: %v\n", err)
		return nil, fmt.Errorf("Failed to search memories")
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
		fmt.Fprintf(os.Stderr, "[MCP ERROR] Failed to list memories: %v\n", err)
		return nil, fmt.Errorf("Failed to list memories")
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
		fmt.Fprintf(os.Stderr, "[MCP ERROR] Failed to list entities: %v\n", err)
		return nil, fmt.Errorf("Failed to list entities")
	}

	if len(entities) == 0 {
		return "No entities found.", nil
	}

	data, _ := json.MarshalIndent(entities, "", "  ")
	return string(data), nil
}

func writeJSONError(w http.ResponseWriter, status int, safeMsg string, internal error) {
	if internal != nil {
		fmt.Fprintf(os.Stderr, "[HTTP ERROR] %s: %v\n", safeMsg, internal)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": safeMsg})
}

func (s *Server) StartHTTPServer(port int) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	srv := &http.Server{
		Addr:    addr,
		Handler: s.httpMux(),
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to bind to %s: %w", addr, err)
	}
	fmt.Fprintf(os.Stderr, "⚡ Symaira Memory API Listening on http://%s\n", addr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

func (s *Server) enableCORS(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	allowed := false
	for _, o := range s.allowedOrigins {
		if matchOrigin(origin, o) {
			allowed = true
			break
		}
	}
	if !allowed {
		if origin != "" {
			http.Error(w, `{"error":"origin not allowed"}`, http.StatusForbidden)
			return true
		}
	} else {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return true
	}
	return false
}

func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) (*security.JWTPayload, bool) {
	if s.jwts == nil {
		return nil, true
	}
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		http.Error(w, `{"error":"missing or invalid Authorization header"}`, http.StatusUnauthorized)
		return nil, false
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	payload, err := s.jwts.VerifyToken(token)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "invalid or expired token", err)
		return nil, false
	}
	return payload, true
}

func (s *Server) requireRole(w http.ResponseWriter, r *http.Request, minRole security.Role) (*security.JWTPayload, bool) {
	payload, ok := s.requireAuth(w, r)
	if !ok {
		return nil, false
	}
	if s.profile != nil {
		if !security.ParseRole(s.profile.Role).CanWrite() && minRole == security.RoleReadWrite {
			writeJSONError(w, http.StatusForbidden, "insufficient permissions: read-only profile", nil)
			return nil, false
		}
		return payload, true
	}
	if payload != nil && payload.Subject != "" && s.db != nil {
		p, err := s.db.GetProfileByName(payload.Subject)
		if err == nil && p != nil {
			if !security.ParseRole(p.Role).CanWrite() && minRole == security.RoleReadWrite {
				writeJSONError(w, http.StatusForbidden, "insufficient permissions: read-only profile", nil)
				return nil, false
			}
		}
	}
	return payload, true
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if s.enableCORS(w, r) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"version": s.version,
		"server":  "symaira-memory",
	})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if s.enableCORS(w, r) {
		return
	}
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var args struct {
		Query  string `json:"query"`
		Scope  string `json:"scope"`
		Limit  int    `json:"limit"`
		Entity string `json:"entity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
		http.Error(w, "Bad request body", http.StatusBadRequest)
		return
	}

	if args.Limit <= 0 {
		args.Limit = 5
	}

	var entityID string
	if args.Entity != "" {
		entity, err := s.db.ResolveEntity(args.Entity)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "Failed to resolve entity", err)
			return
		}
		if entity == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("entity not found: %s", args.Entity)})
			return
		}
		entityID = entity.ID
	}

	queryVector := s.embeddings.GenerateVector(args.Query)
	results, err := s.db.SearchMemoriesFiltered(queryVector, args.Scope, args.Limit, entityID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Search failed", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func (s *Server) handleSet(w http.ResponseWriter, r *http.Request) {
	if s.enableCORS(w, r) {
		return
	}
	payload, ok := s.requireRole(w, r, security.RoleReadWrite)
	if !ok {
		return
	}
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var args struct {
		Content   string            `json:"content"`
		Scope     string            `json:"scope"`
		Metadata  map[string]string `json:"metadata"`
		SessionID string            `json:"session_id"`
		Entities  []string          `json:"entities"`
	}
	if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
		http.Error(w, "Bad request body", http.StatusBadRequest)
		return
	}

	author := "api"
	if payload != nil && payload.Subject != "" {
		author = payload.Subject
	}
	attr := memory.Attribution{
		Author:    author,
		SessionID: args.SessionID,
	}

	m, _, err := memory.Store(s.db, s.embeddings, s.extractor, args.Content, args.Scope, args.Metadata, s.piiEnabled, attr, args.Entities)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to save memory", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
		"id":     m.ID,
	})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	if s.enableCORS(w, r) {
		return
	}
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	scope := r.URL.Query().Get("scope")

	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 1000 {
		limit = 1000
	}

	memories, err := s.db.ListMemoriesLite(scope, 0, limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to list memories", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(memories)
}

func (s *Server) handleSyncChanges(w http.ResponseWriter, r *http.Request) {
	if s.enableCORS(w, r) {
		return
	}
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var since time.Time
	sinceStr := r.URL.Query().Get("since")
	if sinceStr != "" {
		parsed, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid since parameter; expected RFC3339"})
			return
		}
		since = parsed
	}

	cursorStr := r.URL.Query().Get("cursor")
	if cursorStr != "" {
		decoded, err := base64.StdEncoding.DecodeString(cursorStr)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid cursor parameter"})
			return
		}
		parsed, err := time.Parse(time.RFC3339Nano, string(decoded))
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid cursor format"})
			return
		}
		since = parsed
	}

	limit := 500
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 10000 {
		limit = 10000
	}

	var memories []*db.Memory
	var err error
	memories, err = s.db.GetMemoriesSinceCursor(since, limit+1)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to fetch changes", err)
		return
	}

	var nextCursor string
	if len(memories) > limit {
		memories = memories[:limit]
		last := memories[len(memories)-1]
		nextCursor = base64.StdEncoding.EncodeToString([]byte(last.UpdatedAt.Format(time.RFC3339Nano)))
	}

	resp := map[string]interface{}{
		"memories":    memories,
		"server_time": time.Now().UTC().Format(time.RFC3339),
	}
	if nextCursor != "" {
		resp["next_cursor"] = nextCursor
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleSyncApply(w http.ResponseWriter, r *http.Request) {
	if s.enableCORS(w, r) {
		return
	}
	payload, ok := s.requireRole(w, r, security.RoleReadWrite)
	if !ok {
		return
	}
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Memories []*db.Memory `json:"memories"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Bad request body", http.StatusBadRequest)
		return
	}

	var applied, skipped, skippedInvalidScope, skippedInvalidID int
	actor := "api"
	if payload != nil && payload.Subject != "" {
		actor = payload.Subject
	}
	for _, m := range body.Memories {
		if m.ID == "" {
			skipped++
			continue
		}
		if _, err := uuid.Parse(m.ID); err != nil {
			skippedInvalidID++
			continue
		}
		if err := security.ValidateScope(m.Scope); err != nil {
			skippedInvalidScope++
			continue
		}
		if s.piiEnabled {
			m.Content = security.Redact(m.Content)
			m.Metadata = security.RedactMap(m.Metadata)
		}
		isNew, err := s.db.UpsertMemoryIfNewer(m)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "Failed to apply memory", err)
			return
		}
		if isNew {
			applied++
		} else {
			skipped++
		}
	}

	_ = s.db.LogAudit("sync.apply", "", "", "", actor,
		fmt.Sprintf("applied=%d skipped=%d invalidScope=%d invalidID=%d", applied, skipped, skippedInvalidScope, skippedInvalidID))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{
		"applied":             applied,
		"skipped":             skipped,
		"skippedInvalidScope": skippedInvalidScope,
		"skippedInvalidID":    skippedInvalidID,
	})
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	if s.enableCORS(w, r) {
		return
	}
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing required parameter: id"})
		return
	}

	m, err := s.db.GetMemory(id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to fetch memory", err)
		return
	}
	if m == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(m)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	if s.enableCORS(w, r) {
		return
	}
	if _, ok := s.requireRole(w, r, security.RoleReadWrite); !ok {
		return
	}
	if r.Method != "DELETE" && r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing required parameter: id"})
		return
	}

	m, err := s.db.GetMemory(id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to fetch memory", err)
		return
	}
	if m == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		return
	}

	if err := s.db.DeleteMemory(id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to delete memory", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"deleted": true})
}

func (s *Server) handleRules(w http.ResponseWriter, r *http.Request) {
	if s.enableCORS(w, r) {
		return
	}
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	scope := r.URL.Query().Get("scope")
	rules, err := s.db.ListRules(scope)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to list rules", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"rules": rules})
}

func (s *Server) handleEntities(w http.ResponseWriter, r *http.Request) {
	if s.enableCORS(w, r) {
		return
	}
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	entities, err := s.db.ListEntities()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to list entities", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"entities": entities})
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(web.IndexHTML())
		return
	}
	fileServer := http.FileServer(http.FS(web.StaticFS()))
	fileServer.ServeHTTP(w, r)
}

func (s *Server) httpMux() http.Handler {
	mux := http.NewServeMux()

	routes := []struct {
		pattern string
		handler http.HandlerFunc
	}{
		{"/api/status", s.handleStatus},
		{"/api/search", s.handleSearch},
		{"/api/set", s.handleSet},
		{"/api/list", s.handleList},
		{"/api/sync/changes", s.handleSyncChanges},
		{"/api/sync/apply", s.handleSyncApply},
		{"/api/get", s.handleGet},
		{"/api/delete", s.handleDelete},
		{"/api/rules", s.handleRules},
		{"/api/entities", s.handleEntities},
		{"/", s.handleStatic},
	}
	for _, rt := range routes {
		mux.HandleFunc(rt.pattern, rt.handler)
	}

	classify := func(r *http.Request) string {
		if strings.HasPrefix(r.URL.Path, "/api/token") || strings.HasPrefix(r.URL.Path, "/api/login") {
			return "auth"
		}
		return "data"
	}

	var handler http.Handler = mux
	handler = securityHeadersHandler(handler)
	return RateLimitMiddleware(s.rateLimiter, handler, classify)
}

func matchOrigin(origin, pattern string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "://*") {
		scheme := strings.TrimSuffix(pattern, "://*")
		return strings.HasPrefix(origin, scheme+"://")
	}
	return origin == pattern
}

// securityHeadersHandler adds Content-Security-Policy, X-Content-Type-Options,
// and X-Frame-Options headers to all HTTP responses.
func securityHeadersHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/danieljustus/symaira-memory/internal/instructions"
	"github.com/danieljustus/symaira-memory/internal/memory"
	"github.com/danieljustus/symaira-memory/internal/security"
	"github.com/danieljustus/symaira-memory/internal/web"
)

// JSON-RPC 2.0 structures
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Model Context Protocol specific structures
type CallToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type ToolResponse struct {
	Content []ToolContent `json:"content"`
}

type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Server holds dependencies for running the stdio server.
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

// NewServer configures a new Server instance.
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

// SetPIIEnabled controls whether memory content is redacted for PII before persistence.
func (s *Server) SetPIIEnabled(enabled bool) {
	s.piiEnabled = enabled
}

// SetAllowedOrigins overrides the default allowed CORS origins.
func (s *Server) SetAllowedOrigins(origins []string) {
	s.allowedOrigins = origins
}

// SetProfile assigns the active agent profile for role-based access control.
func (s *Server) SetProfile(p *db.Profile) {
	s.profile = p
}

// Serve reads JSON-RPC 2.0 lines from stdin, processes them, and writes responses to stdout.
func (s *Server) Serve(ctx context.Context) error {
	log.SetOutput(os.Stderr)
	log.Println("Symaira Memory MCP Server starting...")

	reader := bufio.NewReader(os.Stdin)

	for {
		select {
		case <-ctx.Done():
			log.Println("MCP Server shutting down gracefully.")
			return nil
		default:
		}

		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				log.Println("MCP Client disconnected.")
				return nil
			}
			return fmt.Errorf("read error: %w", err)
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendError(nil, -32700, "Parse error")
			continue
		}

		s.handleRequest(&req)
	}
}

func (s *Server) handleRequest(req *JSONRPCRequest) {
	switch req.Method {
	case "initialize":
		res := map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"serverInfo": map[string]string{
				"name":    "symaira-memory",
				"version": s.version,
			},
			"instructions": instructions.Text(s.version),
		}
		s.sendResult(req.ID, res)

	case "tools/list":
		tools := []Tool{
			{
				Name:        "memory_get",
				Description: "Retrieve a specific memory by its unique ID.",
				InputSchema: InputSchema{
					Type: "object",
					Properties: map[string]Property{
						"id": {Type: "string", Description: "Unique memory UUID"},
					},
					Required: []string{"id"},
				},
			},
			{
				Name:        "memory_set",
				Description: "Save a new persistent memory or fact. Use this tool autonomously when the user expresses a clear preference, constraint, architectural decision, or guideline that should persist across sessions.",
				InputSchema: InputSchema{
					Type: "object",
					Properties: map[string]Property{
						"content":    {Type: "string", Description: "The text content or fact to remember (e.g., 'User prefers TypeScript for script tasks' or 'API uses port 8080'). Keep it concise and objective."},
						"scope":      {Type: "string", Description: "Scope level: 'global' (default, for general user settings), 'project' (highly recommended for folder-specific codebases; auto-resolves project name using .symmemory.toml or .git in CWD), 'agent', 'user', or 'session'"},
						"metadata":   {Type: "string", Description: "Optional JSON metadata key-value string (e.g., '{\"source\": \"claude-agent\"}')"},
						"session_id": {Type: "string", Description: "Optional session ID for provenance tracking (e.g., the current chat/conversation session identifier)"},
						"entities":   {Type: "string", Description: "Optional comma-separated entity names to link (e.g., 'Irene,Premium BnB'). Entities are auto-created if they don't exist."},
					},
					Required: []string{"content"},
				},
			},
			{
				Name:        "memory_search",
				Description: "Perform a semantic vector similarity search on stored memories. Always use this tool at the start of a session or task to retrieve relevant past design decisions, user preferences, and project guidelines.",
				InputSchema: InputSchema{
					Type: "object",
					Properties: map[string]Property{
						"query":  {Type: "string", Description: "The natural language query or semantic term (e.g., 'database port' or 'language preference')"},
						"scope":  {Type: "string", Description: "Optional scope level filter ('global', 'project', 'agent', 'user', 'session')"},
						"limit":  {Type: "string", Description: "Optional maximum number of search results to return (default 5)"},
						"entity": {Type: "string", Description: "Optional entity name filter — only returns memories linked to this entity"},
					},
					Required: []string{"query"},
				},
			},
		{
			Name:        "memory_list",
			Description: "List all memories currently stored in the database. Useful for debugging or displaying stored context lists.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"scope": {Type: "string", Description: "Optional scope level filter ('global', 'project', 'agent', 'user', 'session')"},
					"limit": {Type: "string", Description: "Optional maximum number of memories to return (default 100, max 1000)"},
				},
			},
		},
			{
				Name:        "entity_list",
				Description: "List all known entities (people, projects, organizations). Use this to discover which entities exist before linking memories or filtering searches.",
				InputSchema: InputSchema{
					Type:       "object",
					Properties: map[string]Property{},
				},
			},
		}
		s.sendResult(req.ID, map[string]interface{}{"tools": tools})

	case "tools/call":
		var params CallToolParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.sendError(req.ID, -32602, "Invalid params: expected object with 'name' and 'arguments'", map[string]string{"detail": err.Error()})
			return
		}
		if params.Name == "" {
			s.sendError(req.ID, -32602, "Invalid params: 'name' is required")
			return
		}
		s.handleToolCall(req.ID, &params)

	default:
		s.sendError(req.ID, -32601, "Method not found")
	}
}

func (s *Server) handleToolCall(reqID json.RawMessage, params *CallToolParams) {
	switch params.Name {
	case "memory_get":
		var args struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.sendError(reqID, -32602, "Invalid arguments for 'memory_get': failed to parse arguments", map[string]string{"detail": err.Error()})
			return
		}
		if args.ID == "" {
			s.sendError(reqID, -32602, "Invalid arguments for 'memory_get': 'id' is required")
			return
		}

		m, err := s.db.GetMemory(args.ID)
		if err != nil {
			s.sendToolError(reqID, "Failed to fetch memory", err)
			return
		}

		if m == nil {
			s.sendToolResponse(reqID, "Memory not found", false)
			return
		}

		bytes, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			s.sendToolError(reqID, "Failed to encode memory data", err)
			return
		}
		s.sendToolResponse(reqID, string(bytes), false)

	case "memory_set":
		if s.profile != nil && !security.ParseRole(s.profile.Role).CanWrite() {
			s.sendToolResponse(reqID, "Permission denied: profile role is read-only", true)
			return
		}

		var args struct {
			Content   string `json:"content"`
			Scope     string `json:"scope"`
			Metadata  string `json:"metadata"`
			SessionID string `json:"session_id"`
			Entities  string `json:"entities"`
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.sendError(reqID, -32602, "Invalid arguments for 'memory_set': failed to parse arguments", map[string]string{"detail": err.Error()})
			return
		}
		if args.Content == "" {
			s.sendError(reqID, -32602, "Invalid arguments for 'memory_set': 'content' is required")
			return
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
			s.sendToolResponse(reqID, err.Error(), true)
			return
		}

		responseMsg := memory.FormatStoreSuccess(m, extractedStr)
		s.sendToolResponse(reqID, responseMsg, false)

	case "memory_search":
		var args struct {
			Query  string `json:"query"`
			Scope  string `json:"scope"`
			Limit  string `json:"limit"`
			Entity string `json:"entity"`
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.sendError(reqID, -32602, "Invalid arguments for 'memory_search': failed to parse arguments", map[string]string{"detail": err.Error()})
			return
		}
		if args.Query == "" {
			s.sendError(reqID, -32602, "Invalid arguments for 'memory_search': 'query' is required")
			return
		}

		limit := 5
		if args.Limit != "" {
			if l, err := strconv.Atoi(args.Limit); err == nil && l > 0 {
				limit = l
			}
		}

		var entityID string
		if args.Entity != "" {
			entity, err := s.db.ResolveEntity(args.Entity)
			if err != nil {
				s.sendToolError(reqID, "Failed to resolve entity", err)
				return
			}
			if entity == nil {
				s.sendToolResponse(reqID, fmt.Sprintf("Entity not found: %s", args.Entity), true)
				return
			}
			entityID = entity.ID
		}

		queryVector := s.embeddings.GenerateVector(args.Query)
		results, err := s.db.SearchMemoriesFiltered(queryVector, args.Scope, limit, entityID)
		if err != nil {
			s.sendToolError(reqID, "Failed to search memories", err)
			return
		}

		if len(results) == 0 {
			s.sendToolResponse(reqID, "No relevant memories found.", false)
			return
		}

		bytes, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			s.sendToolError(reqID, "Failed to encode search results", err)
			return
		}
		s.sendToolResponse(reqID, string(bytes), false)

	case "memory_list":
		var args struct {
			Scope string `json:"scope"`
			Limit string `json:"limit"`
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.sendError(reqID, -32602, "Invalid arguments for 'memory_list': failed to parse arguments", map[string]string{"detail": err.Error()})
			return
		}

		limit := 100
		if args.Limit != "" {
			if l, err := strconv.Atoi(args.Limit); err == nil {
				limit = l
			}
		}
		if limit < 1 {
			limit = 1
		}
		if limit > 1000 {
			limit = 1000
		}

		memories, err := s.db.ListMemoriesLite(args.Scope, 0, limit)
		if err != nil {
			s.sendToolError(reqID, "Failed to list memories", err)
			return
		}

		if len(memories) == 0 {
			s.sendToolResponse(reqID, "Memory store is empty.", false)
			return
		}

		bytes, err := json.MarshalIndent(memories, "", "  ")
		if err != nil {
			s.sendToolError(reqID, "Failed to encode memory list", err)
			return
		}
		s.sendToolResponse(reqID, string(bytes), false)

	case "entity_list":
		entities, err := s.db.ListEntities()
		if err != nil {
			s.sendToolError(reqID, "Failed to list entities", err)
			return
		}

		if len(entities) == 0 {
			s.sendToolResponse(reqID, "No entities found.", false)
			return
		}

		bytes, err := json.MarshalIndent(entities, "", "  ")
		if err != nil {
			s.sendToolError(reqID, "Failed to encode entity list", err)
			return
		}
		s.sendToolResponse(reqID, string(bytes), false)

	default:
		s.sendError(reqID, -32601, fmt.Sprintf("Tool not implemented: '%s'", params.Name))
	}
}

func (s *Server) sendResult(id json.RawMessage, result interface{}) {
	res := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	s.sendResponse(&res)
}

func (s *Server) sendError(id json.RawMessage, code int, message string, data ...interface{}) {
	res := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
		},
	}
	if len(data) > 0 {
		res.Error.Data = data[0]
	}
	s.sendResponse(&res)
}

func (s *Server) sendToolResponse(id json.RawMessage, text string, isError bool) {
	prefix := ""
	if isError {
		prefix = "[ERROR] "
	}
	res := ToolResponse{
		Content: []ToolContent{
			{
				Type: "text",
				Text: prefix + text,
			},
		},
	}
	s.sendResult(id, res)
}

// sendToolError sends a user-safe error message to the agent while logging
// the full internal error details to stderr for diagnostics.
func (s *Server) sendToolError(id json.RawMessage, safeMsg string, internalErr error) {
	fmt.Fprintf(os.Stderr, "[MCP ERROR] %s: %v\n", safeMsg, internalErr)
	s.sendToolResponse(id, safeMsg, true)
}

func (s *Server) sendResponse(res *JSONRPCResponse) {
	bytes, err := json.Marshal(res)
	if err != nil {
		log.Printf("Marshal error: %v\n", err)
		return
	}
	bytes = append(bytes, '\n')
	os.Stdout.Write(bytes)
	os.Stdout.Sync()
}

// writeJSONError writes a user-safe JSON error body and logs the full internal
// error to stderr for diagnostics. The HTTP response body never reveals
// internal state (DB errors, file paths, SQL details, etc).
func writeJSONError(w http.ResponseWriter, status int, safeMsg string, internal error) {
	if internal != nil {
		fmt.Fprintf(os.Stderr, "[HTTP ERROR] %s: %v\n", safeMsg, internal)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": safeMsg})
}

// StartHTTPServer launches a local HTTP listener exposing REST endpoints for the browser extension.
func (s *Server) StartHTTPServer(port int) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	log.Printf("⚡ Symaira Memory API Listening on http://%s\n", addr)
	return http.ListenAndServe(addr, s.httpMux())
}

// enableCORS handles CORS headers for extension origin requests.
// Returns true if the request was fully handled (OPTIONS preflight or forbidden origin).
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
		// When origin is missing (same-origin) or not allowed, omit the header
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

// requireAuth validates the JWT Bearer token. Returns false and writes 401 on failure.
// When authentication succeeds, the verified JWT payload is returned for downstream use.
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

// requireRole checks that the authenticated subject has at least the given role.
// If no profile is configured on the server, the check passes (backward compatible).
// If a profile is set, the JWT subject is resolved to a profile and its role is checked.
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

// handleStatus serves GET /api/status.
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

// handleSearch serves POST /api/search.
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

// handleSet serves POST /api/set.
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

// handleList serves GET /api/list.
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

// handleSyncChanges serves GET /api/sync/changes.
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

	var memories []*db.Memory
	var err error
	if since.IsZero() {
		memories, err = s.db.ListMemoriesLite("", 0, 100000)
	} else {
		memories, err = s.db.GetMemoriesSince(since)
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to fetch changes", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"memories":    memories,
		"server_time": time.Now().UTC().Format(time.RFC3339),
	})
}

// handleSyncApply serves POST /api/sync/apply.
func (s *Server) handleSyncApply(w http.ResponseWriter, r *http.Request) {
	if s.enableCORS(w, r) {
		return
	}
	if _, ok := s.requireRole(w, r, security.RoleReadWrite); !ok {
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

	var applied, skipped, skippedInvalidScope int
	for _, m := range body.Memories {
		if m.ID == "" {
			skipped++
			continue
		}
		if err := security.ValidateScope(m.Scope); err != nil {
			skippedInvalidScope++
			continue
		}
		if s.piiEnabled {
			m.Content = security.Redact(m.Content)
		}
		ok, err := s.db.UpsertMemoryIfNewer(m)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "Failed to apply memory", err)
			return
		}
		if ok {
			applied++
		} else {
			skipped++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{
		"applied":             applied,
		"skipped":             skipped,
		"skippedInvalidScope": skippedInvalidScope,
	})
}

// handleGet serves GET /api/get?id=<memory-id>.
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

// handleDelete serves DELETE|POST /api/delete?id=<memory-id>.
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

// handleRules serves GET /api/rules.
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

// handleEntities serves GET /api/entities.
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

// handleStatic serves the web console and embedded static assets.
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(web.IndexHTML())
		return
	}
	fileServer := http.FileServer(http.FS(web.StaticFS()))
	fileServer.ServeHTTP(w, r)
}

// httpMux builds the HTTP request multiplexer with table-driven route registration.
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

	return RateLimitMiddleware(s.rateLimiter, mux, classify)
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

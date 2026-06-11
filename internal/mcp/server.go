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
	Code    int    `json:"code"`
	Message string `json:"message"`
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
						"content":  {Type: "string", Description: "The text content or fact to remember (e.g., 'User prefers TypeScript for script tasks' or 'API uses port 8080'). Keep it concise and objective."},
						"scope":    {Type: "string", Description: "Scope level: 'global' (default, for general user settings), 'project' (highly recommended for folder-specific codebases; auto-resolves project name using .symmemory.toml or .git in CWD), 'agent', 'user', or 'session'"},
						"metadata": {Type: "string", Description: "Optional JSON metadata key-value string (e.g., '{\"source\": \"claude-agent\"}')"},
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
						"query": {Type: "string", Description: "The natural language query or semantic term (e.g., 'database port' or 'language preference')"},
						"scope": {Type: "string", Description: "Optional scope level filter ('global', 'project', 'agent', 'user', 'session')"},
						"limit": {Type: "string", Description: "Optional maximum number of search results to return (default 5)"},
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
					},
				},
			},
		}
		s.sendResult(req.ID, map[string]interface{}{"tools": tools})

	case "tools/call":
		var params CallToolParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.sendError(req.ID, -32602, "Invalid params")
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
			s.sendError(reqID, -32602, "Invalid arguments")
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
		var args struct {
			Content  string `json:"content"`
			Scope    string `json:"scope"`
			Metadata string `json:"metadata"`
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.sendError(reqID, -32602, "Invalid arguments")
			return
		}

		meta := make(map[string]string)
		if args.Metadata != "" {
			_ = json.Unmarshal([]byte(args.Metadata), &meta)
		}

		m, extractedStr, err := memory.Store(s.db, s.embeddings, s.extractor, args.Content, args.Scope, meta, s.piiEnabled)
		if err != nil {
			s.sendToolResponse(reqID, err.Error(), true)
			return
		}

		responseMsg := memory.FormatStoreSuccess(m, extractedStr)
		s.sendToolResponse(reqID, responseMsg, false)

	case "memory_search":
		var args struct {
			Query string `json:"query"`
			Scope string `json:"scope"`
			Limit string `json:"limit"`
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.sendError(reqID, -32602, "Invalid arguments")
			return
		}

		limit := 5
		if args.Limit != "" {
			if l, err := strconv.Atoi(args.Limit); err == nil && l > 0 {
				limit = l
			}
		}

		queryVector := s.embeddings.GenerateVector(args.Query)
		results, err := s.db.SearchMemories(queryVector, args.Scope, limit)
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
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.sendError(reqID, -32602, "Invalid arguments")
			return
		}

		memories, err := s.db.ListMemoriesLite(args.Scope, 0, 1000)
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

	default:
		s.sendError(reqID, -32601, "Tool not implemented")
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

func (s *Server) sendError(id json.RawMessage, code int, message string) {
	res := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
		},
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

func (s *Server) httpMux() http.Handler {
	mux := http.NewServeMux()

	// CORS Helper for extension origin requests
	enableCORS := func(w http.ResponseWriter, r *http.Request) bool {
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
	requireAuth := func(w http.ResponseWriter, r *http.Request) bool {
		if s.jwts == nil {
			return true
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, `{"error":"missing or invalid Authorization header"}`, http.StatusUnauthorized)
			return false
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		if _, err := s.jwts.VerifyToken(token); err != nil {
			writeJSONError(w, http.StatusUnauthorized, "invalid or expired token", err)
			return false
		}
		return true
	}

	// GET /api/status
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		if enableCORS(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "healthy",
			"version": s.version,
			"server":  "symaira-memory",
		})
	})

	// POST /api/search
	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		if enableCORS(w, r) {
			return
		}
		if !requireAuth(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var args struct {
			Query string `json:"query"`
			Scope string `json:"scope"`
			Limit int    `json:"limit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
			http.Error(w, "Bad request body", http.StatusBadRequest)
			return
		}

		if args.Limit <= 0 {
			args.Limit = 5
		}

		queryVector := s.embeddings.GenerateVector(args.Query)
		results, err := s.db.SearchMemories(queryVector, args.Scope, args.Limit)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "Search failed", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	})

	// POST /api/set
	mux.HandleFunc("/api/set", func(w http.ResponseWriter, r *http.Request) {
		if enableCORS(w, r) {
			return
		}
		if !requireAuth(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var args struct {
			Content  string            `json:"content"`
			Scope    string            `json:"scope"`
			Metadata map[string]string `json:"metadata"`
		}
		if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
			http.Error(w, "Bad request body", http.StatusBadRequest)
			return
		}

		m, _, err := memory.Store(s.db, s.embeddings, s.extractor, args.Content, args.Scope, args.Metadata, s.piiEnabled)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "Failed to save memory", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "success",
			"id":     m.ID,
		})
	})

	// GET /api/list
	mux.HandleFunc("/api/list", func(w http.ResponseWriter, r *http.Request) {
		if enableCORS(w, r) {
			return
		}
		if !requireAuth(w, r) {
			return
		}
		scope := r.URL.Query().Get("scope")
		memories, err := s.db.ListMemoriesLite(scope, 0, 1000)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "Failed to list memories", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(memories)
	})

	// GET /api/sync/changes
	mux.HandleFunc("/api/sync/changes", func(w http.ResponseWriter, r *http.Request) {
		if enableCORS(w, r) {
			return
		}
		if !requireAuth(w, r) {
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
	})

	// POST /api/sync/apply
	mux.HandleFunc("/api/sync/apply", func(w http.ResponseWriter, r *http.Request) {
		if enableCORS(w, r) {
			return
		}
		if !requireAuth(w, r) {
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
	})

	// GET /api/get?id=<memory-id>
	mux.HandleFunc("/api/get", func(w http.ResponseWriter, r *http.Request) {
		if enableCORS(w, r) {
			return
		}
		if !requireAuth(w, r) {
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
	})

	// DELETE|POST /api/delete?id=<memory-id>
	mux.HandleFunc("/api/delete", func(w http.ResponseWriter, r *http.Request) {
		if enableCORS(w, r) {
			return
		}
		if !requireAuth(w, r) {
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
	})

	// GET /api/rules
	mux.HandleFunc("/api/rules", func(w http.ResponseWriter, r *http.Request) {
		if enableCORS(w, r) {
			return
		}
		if !requireAuth(w, r) {
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
	})

	fileServer := http.FileServer(http.FS(web.StaticFS()))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(web.IndexHTML())
			return
		}
		fileServer.ServeHTTP(w, r)
	})

	return mux
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

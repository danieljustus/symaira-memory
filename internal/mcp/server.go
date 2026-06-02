package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/danieljustus/symaira-memory/internal/security"
	"github.com/google/uuid"
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
	db         *db.DB
	extractor  *extractor.PatternExtractor
	embeddings *extractor.EmbeddingsGenerator
	jwts       *security.JWTProvider
}

// NewServer configures a new Server instance.
func NewServer(database *db.DB, jwtProvider *security.JWTProvider) *Server {
	return &Server{
		db:         database,
		extractor:  extractor.NewPatternExtractor(),
		embeddings: extractor.NewEmbeddingsGenerator(),
		jwts:       jwtProvider,
	}
}

// Serve reads JSON-RPC 2.0 lines from stdin, processes them, and writes responses to stdout.
func (s *Server) Serve() {
	// Re-route normal standard logger output to stderr to prevent stdio protocol pollution!
	log.SetOutput(os.Stderr)
	log.Println("Symaira Memory MCP Server starting...")

	reader := bufio.NewReader(os.Stdin)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				log.Println("MCP Client disconnected.")
				break
			}
			log.Printf("Read error: %v\n", err)
			continue
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
				"version": "0.1.0",
			},
		}
		s.sendResult(req.ID, res)

	case "tools/list":
		tools := []Tool{
			{
				Name:        "memory_get",
				Description: "Get a specific memory by its unique ID",
				InputSchema: InputSchema{
					Type: "object",
					Properties: map[string]Property{
						"id": {Type: "string", Description: "Unique memory ID"},
					},
					Required: []string{"id"},
				},
			},
			{
				Name:        "memory_set",
				Description: "Save a new memory or fact into the persistent memory store",
				InputSchema: InputSchema{
					Type: "object",
					Properties: map[string]Property{
						"content":  {Type: "string", Description: "The text content or fact to remember"},
						"scope":    {Type: "string", Description: "Scope level: global (default), project, agent, user, session"},
						"metadata": {Type: "string", Description: "Optional JSON metadata string"},
					},
					Required: []string{"content"},
				},
			},
			{
				Name:        "memory_search",
				Description: "Semantic search of memories using vector similarity comparison",
				InputSchema: InputSchema{
					Type: "object",
					Properties: map[string]Property{
						"query": {Type: "string", Description: "The text query or semantic match string"},
						"scope": {Type: "string", Description: "Optional scope level filter"},
						"limit": {Type: "string", Description: "Optional search limit (default 5)"},
					},
					Required: []string{"query"},
				},
			},
			{
				Name:        "memory_list",
				Description: "List all memories currently stored in the database",
				InputSchema: InputSchema{
					Type: "object",
					Properties: map[string]Property{
						"scope": {Type: "string", Description: "Optional scope level filter"},
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

		mems, err := s.db.ListMemories("")
		if err != nil {
			s.sendToolResponse(reqID, fmt.Sprintf("Error fetching memory: %v", err), true)
			return
		}

		var target *db.Memory
		for _, m := range mems {
			if m.ID == args.ID {
				target = m
				break
			}
		}

		if target == nil {
			s.sendToolResponse(reqID, "Memory not found", false)
			return
		}

		bytes, _ := json.MarshalIndent(target, "", "  ")
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

		// Security Integration: PII Guard Redaction
		piiGuard := security.NewPIIGuard()
		cleanContent := piiGuard.Redact(args.Content)

		scope := args.Scope
		if scope == "" {
			scope = "global"
		}

		meta := make(map[string]string)
		if args.Metadata != "" {
			_ = json.Unmarshal([]byte(args.Metadata), &meta)
		}

		// Security Integration: Active Project Scope detection
		if scope == "project" {
			detector := security.NewProjectScopeDetector()
			projName := detector.DetectActiveProject()
			meta["project_name"] = projName
		}

		// Calculate semantic vector embedding for the content
		vector := s.embeddings.GenerateVector(cleanContent)

		// Create and save core memory
		memID := uuid.New().String()
		m := &db.Memory{
			ID:        memID,
			Content:   cleanContent,
			Scope:     scope,
			Metadata:  meta,
			Embedding: vector,
		}

		if err := s.db.SaveMemory(m); err != nil {
			s.sendToolResponse(reqID, fmt.Sprintf("Database save failure: %v", err), true)
			return
		}

		// Also execute pattern extractor offline to see if there are any secondary facts we can automatically extract!
		extractedFacts := s.extractor.ExtractFacts(cleanContent)
		var extractedStr []string
		for _, f := range extractedFacts {
			subID := uuid.New().String()
			
			// Redact PII in extracted facts as well
			cleanFactContent := piiGuard.Redact(f.Content)
			
			subVector := s.embeddings.GenerateVector(cleanFactContent)
			
			subMeta := f.Metadata
			if subMeta == nil {
				subMeta = make(map[string]string)
			}
			if scope == "project" {
				subMeta["project_name"] = meta["project_name"]
			}

			subMem := &db.Memory{
				ID:        subID,
				Content:   cleanFactContent,
				Scope:     scope,
				Metadata:  subMeta,
				Embedding: subVector,
			}
			if err := s.db.SaveMemory(subMem); err == nil {
				extractedStr = append(extractedStr, fmt.Sprintf("  - [Fact Extracted] %s (ID: %s)", cleanFactContent, subID))
			}
		}

		responseMsg := fmt.Sprintf("Successfully saved memory!\nMemory ID: %s\nContent: %s\nScope: %s", memID, cleanContent, scope)
		if scope == "project" {
			responseMsg += fmt.Sprintf("\nProject: %s", meta["project_name"])
		}
		if len(extractedStr) > 0 {
			responseMsg += "\n\nAdditionally, secondary facts were successfully extracted:\n" + strings.Join(extractedStr, "\n")
		}

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
			s.sendToolResponse(reqID, fmt.Sprintf("Search error: %v", err), true)
			return
		}

		if len(results) == 0 {
			s.sendToolResponse(reqID, "No relevant memories found.", false)
			return
		}

		bytes, _ := json.MarshalIndent(results, "", "  ")
		s.sendToolResponse(reqID, string(bytes), false)

	case "memory_list":
		var args struct {
			Scope string `json:"scope"`
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.sendError(reqID, -32602, "Invalid arguments")
			return
		}

		memories, err := s.db.ListMemories(args.Scope)
		if err != nil {
			s.sendToolResponse(reqID, fmt.Sprintf("List error: %v", err), true)
			return
		}

		if len(memories) == 0 {
			s.sendToolResponse(reqID, "Memory store is empty.", false)
			return
		}

		bytes, _ := json.MarshalIndent(memories, "", "  ")
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
	// If it is an error, prefix text
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

// StartHTTPServer launches a local HTTP listener exposing REST endpoints for the browser extension.
func (s *Server) StartHTTPServer(port int) error {
	mux := http.NewServeMux()

	// CORS Helper for extension origin requests
	enableCORS := func(w http.ResponseWriter, r *http.Request) bool {
		w.Header().Set("Access-Control-Allow-Origin", "*")
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
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusUnauthorized)
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
			"version": "0.1.0",
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
			http.Error(w, fmt.Sprintf("Search failed: %v", err), http.StatusInternalServerError)
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

		scope := args.Scope
		if scope == "" {
			scope = "global"
		}

		vector := s.embeddings.GenerateVector(args.Content)
		memID := uuid.New().String()
		
		m := &db.Memory{
			ID:        memID,
			Content:   args.Content,
			Scope:     scope,
			Metadata:  args.Metadata,
			Embedding: vector,
		}

		if err := s.db.SaveMemory(m); err != nil {
			http.Error(w, fmt.Sprintf("Failed to save: %v", err), http.StatusInternalServerError)
			return
		}

		// Background offline fact extraction
		extracted := s.extractor.ExtractFacts(args.Content)
		for _, f := range extracted {
			subID := uuid.New().String()
			subVector := s.embeddings.GenerateVector(f.Content)
			subMem := &db.Memory{
				ID:        subID,
				Content:   f.Content,
				Scope:     scope,
				Metadata:  f.Metadata,
				Embedding: subVector,
			}
			_ = s.db.SaveMemory(subMem)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "success",
			"id":     memID,
		})
	})

	// GET /api/list
	mux.HandleFunc("/api/list", func(w http.ResponseWriter, r *http.Request) {
		if enableCORS(w, r) {
			return
		}
		scope := r.URL.Query().Get("scope")
		memories, err := s.db.ListMemories(scope)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(memories)
	})

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	log.Printf("⚡ Symaira Memory API Listening on http://%s\n", addr)
	return http.ListenAndServe(addr, mux)
}

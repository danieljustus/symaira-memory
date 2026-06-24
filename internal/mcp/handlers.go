package mcp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/memory"
	"github.com/danieljustus/symaira-memory/internal/security"
	"github.com/danieljustus/symaira-memory/internal/web"
	"github.com/google/uuid"
)

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if s.enableCORS(w, r) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":            "healthy",
		"version":           s.version,
		"server":            "symaira-memory",
		"embedding_backend": s.embeddings.ActiveBackend(),
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

	emb := s.embeddings.GenerateVector(args.Query)
	queryVector := emb.Vector
	results, err := s.db.SearchMemoriesFiltered(queryVector, emb.Source, args.Scope, args.Limit, entityID)
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
	includeEmb := r.URL.Query().Get("include_embeddings") == "true"
	memories, err = s.db.GetMemoriesSinceCursor(since, limit+1, includeEmb)
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

package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/security"
)

// helperDB creates a temporary SQLite database for testing.
func helperDB(t *testing.T) *db.DB {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "symmemory-mcp-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	t.Cleanup(func() { os.Setenv("HOME", oldHome) })

	database, err := db.Open(config.Defaults())
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// helperServer creates a Server backed by a real temp database.
func helperServer(t *testing.T) *Server {
	t.Helper()
	database := helperDB(t)
	jwtProvider, err := security.NewJWTProvider(config.Defaults(), nil)
	if err != nil {
		t.Fatalf("failed to create JWT provider: %v", err)
	}
	return NewServer(database, jwtProvider)
}

// captureStdout captures writes to os.Stdout and restores it after the test.
// Tests that exercise sendResponse need this to capture JSON-RPC output.
func captureResponse(fn func()) string {
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	old := os.Stdout
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

// --------------------------------------------------------------------------
// JSON-RPC Request Parsing
// --------------------------------------------------------------------------

func TestJSONRPCParseValidRequest(t *testing.T) {
	input := []byte(`{"jsonrpc":"2.0","id":"req-1","method":"initialize"}`)
	var req JSONRPCRequest
	if err := json.Unmarshal(input, &req); err != nil {
		t.Fatalf("failed to parse valid request: %v", err)
	}
	if req.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc '2.0', got %q", req.JSONRPC)
	}
	if string(req.ID) != `"req-1"` {
		t.Errorf("expected id 'req-1', got %s", string(req.ID))
	}
	if req.Method != "initialize" {
		t.Errorf("expected method 'initialize', got %q", req.Method)
	}
}

func TestJSONRPCParseRequestWithParams(t *testing.T) {
	input := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"memory_get","arguments":{"id":"abc"}}}`)
	var req JSONRPCRequest
	if err := json.Unmarshal(input, &req); err != nil {
		t.Fatalf("failed to parse request with params: %v", err)
	}
	if string(req.ID) != "1" {
		t.Errorf("expected numeric id '1', got %s", string(req.ID))
	}
	if req.Params == nil {
		t.Fatal("expected non-nil params")
	}
}

func TestJSONRPCParseInvalidJSON(t *testing.T) {
	input := []byte(`not json at all`)
	var req JSONRPCRequest
	err := json.Unmarshal(input, &req)
	if err == nil {
		t.Error("expected parse error for invalid JSON")
	}
}

func TestJSONRPCParseNotification(t *testing.T) {
	// Notifications have no id field
	input := []byte(`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`)
	var req JSONRPCRequest
	if err := json.Unmarshal(input, &req); err != nil {
		t.Fatalf("failed to parse notification: %v", err)
	}
	if req.ID != nil {
		t.Errorf("expected nil id for notification, got %s", string(req.ID))
	}
}

// --------------------------------------------------------------------------
// handleRequest dispatches
// --------------------------------------------------------------------------

func TestHandleRequestInitialize(t *testing.T) {
	s := helperServer(t)
	output := captureResponse(func() {
		s.handleRequest(&JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      json.RawMessage(`"req-1"`),
			Method:  "initialize",
		})
	})

	var res JSONRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &res); err != nil {
		t.Fatalf("failed to parse response: %v\noutput: %s", err, output)
	}
	if res.Error != nil {
		t.Fatalf("unexpected error: %+v", res.Error)
	}

	result, ok := res.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", res.Result)
	}
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("expected protocolVersion '2024-11-05', got %v", result["protocolVersion"])
	}
	serverInfo, ok := result["serverInfo"].(map[string]interface{})
	if !ok {
		t.Fatal("expected serverInfo map")
	}
	if serverInfo["name"] != "symaira-memory" {
		t.Errorf("expected name 'symaira-memory', got %v", serverInfo["name"])
	}
	if serverInfo["version"] != "0.1.0" {
		t.Errorf("expected version '0.1.0', got %v", serverInfo["version"])
	}
}

func TestHandleRequestToolsList(t *testing.T) {
	s := helperServer(t)
	output := captureResponse(func() {
		s.handleRequest(&JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      json.RawMessage(`"req-2"`),
			Method:  "tools/list",
		})
	})

	var res JSONRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &res); err != nil {
		t.Fatalf("failed to parse response: %v\noutput: %s", err, output)
	}
	if res.Error != nil {
		t.Fatalf("unexpected error: %+v", res.Error)
	}

	result, ok := res.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", res.Result)
	}
	tools, ok := result["tools"].([]interface{})
	if !ok {
		t.Fatalf("expected tools array, got %T", result["tools"])
	}

	expectedTools := map[string]bool{
		"memory_get":    false,
		"memory_set":    false,
		"memory_search": false,
		"memory_list":   false,
	}
	for _, toolRaw := range tools {
		tool, ok := toolRaw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := tool["name"].(string)
		if _, exists := expectedTools[name]; exists {
			expectedTools[name] = true
		}
	}
	for name, found := range expectedTools {
		if !found {
			t.Errorf("expected tool %q in tools/list response", name)
		}
	}
}

func TestHandleRequestMethodNotFound(t *testing.T) {
	s := helperServer(t)
	output := captureResponse(func() {
		s.handleRequest(&JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      json.RawMessage(`"req-3"`),
			Method:  "nonexistent/method",
		})
	})

	var res JSONRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &res); err != nil {
		t.Fatalf("failed to parse response: %v\noutput: %s", err, output)
	}
	if res.Error == nil {
		t.Fatal("expected error response for unknown method")
	}
	if res.Error.Code != -32601 {
		t.Errorf("expected error code -32601, got %d", res.Error.Code)
	}
	if res.Error.Message != "Method not found" {
		t.Errorf("expected message 'Method not found', got %q", res.Error.Message)
	}
}

// --------------------------------------------------------------------------
// tools/call routing
// --------------------------------------------------------------------------

func TestHandleToolCallInvalidParams(t *testing.T) {
	s := helperServer(t)
	output := captureResponse(func() {
		s.handleRequest(&JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      json.RawMessage(`"req-4"`),
			Method:  "tools/call",
			Params:  json.RawMessage(`"not-an-object"`),
		})
	})

	var res JSONRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &res); err != nil {
		t.Fatalf("failed to parse response: %v\noutput: %s", err, output)
	}
	if res.Error == nil {
		t.Fatal("expected error response for invalid params")
	}
	if res.Error.Code != -32602 {
		t.Errorf("expected error code -32602, got %d", res.Error.Code)
	}
}

func TestHandleToolCallUnknownTool(t *testing.T) {
	s := helperServer(t)
	params := CallToolParams{Name: "nonexistent_tool", Arguments: json.RawMessage(`{}`)}
	paramsJSON, _ := json.Marshal(params)
	output := captureResponse(func() {
		s.handleRequest(&JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      json.RawMessage(`"req-5"`),
			Method:  "tools/call",
			Params:  paramsJSON,
		})
	})

	var res JSONRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &res); err != nil {
		t.Fatalf("failed to parse response: %v\noutput: %s", err, output)
	}
	if res.Error == nil {
		t.Fatal("expected error response for unknown tool")
	}
	if res.Error.Code != -32601 {
		t.Errorf("expected error code -32601, got %d", res.Error.Code)
	}
}

// --------------------------------------------------------------------------
// Tool handler: memory_get
// --------------------------------------------------------------------------

func TestToolMemoryGetMissingArgs(t *testing.T) {
	s := helperServer(t)
	params := CallToolParams{Name: "memory_get", Arguments: json.RawMessage(`{}`)}
	paramsJSON, _ := json.Marshal(params)
	output := captureResponse(func() {
		s.handleRequest(&JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      json.RawMessage(`"req-6"`),
			Method:  "tools/call",
			Params:  paramsJSON,
		})
	})

	var res JSONRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &res); err != nil {
		t.Fatalf("failed to parse response: %v\noutput: %s", err, output)
	}
	if res.Error != nil {
		t.Fatalf("unexpected error: %+v", res.Error)
	}
}

func TestToolMemoryGetNotFound(t *testing.T) {
	s := helperServer(t)
	args, _ := json.Marshal(map[string]string{"id": "nonexistent"})
	params := CallToolParams{Name: "memory_get", Arguments: args}
	paramsJSON, _ := json.Marshal(params)
	output := captureResponse(func() {
		s.handleRequest(&JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      json.RawMessage(`"req-7"`),
			Method:  "tools/call",
			Params:  paramsJSON,
		})
	})

	var res JSONRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &res); err != nil {
		t.Fatalf("failed to parse response: %v\noutput: %s", err, output)
	}
	if res.Error != nil {
		t.Fatalf("unexpected error: %+v", res.Error)
	}
	// Should return a ToolResponse with "Memory not found" text.
	result, ok := res.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result")
	}
	content, ok := result["content"].([]interface{})
	if !ok || len(content) == 0 {
		t.Fatal("expected content array")
	}
	item := content[0].(map[string]interface{})
	if text, _ := item["text"].(string); !strings.Contains(text, "Memory not found") {
		t.Errorf("expected 'Memory not found', got %q", text)
	}
}

func TestToolMemoryGetSuccess(t *testing.T) {
	s := helperServer(t)

	// Save a memory first
	m := &db.Memory{
		ID:        "test-mem-1",
		Content:   "User prefers Go for backend",
		Scope:     "global",
		Metadata:  map[string]string{"source": "test"},
		Embedding: []float32{0.1, 0.2, 0.3},
	}
	if err := s.db.SaveMemory(m); err != nil {
		t.Fatalf("failed to save test memory: %v", err)
	}

	args, _ := json.Marshal(map[string]string{"id": "test-mem-1"})
	params := CallToolParams{Name: "memory_get", Arguments: args}
	paramsJSON, _ := json.Marshal(params)
	output := captureResponse(func() {
		s.handleRequest(&JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      json.RawMessage(`"req-8"`),
			Method:  "tools/call",
			Params:  paramsJSON,
		})
	})

	var res JSONRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &res); err != nil {
		t.Fatalf("failed to parse response: %v\noutput: %s", err, output)
	}
	if res.Error != nil {
		t.Fatalf("unexpected error: %+v", res.Error)
	}

	// The result is a ToolResponse with a JSON-serialized memory
	result, ok := res.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result")
	}
	content, ok := result["content"].([]interface{})
	if !ok || len(content) == 0 {
		t.Fatal("expected content array")
	}
	item := content[0].(map[string]interface{})
	text, _ := item["text"].(string)

	var mem db.Memory
	if err := json.Unmarshal([]byte(text), &mem); err != nil {
		t.Fatalf("failed to unmarshal memory from response: %v\ntext: %s", err, text)
	}
	if mem.ID != "test-mem-1" {
		t.Errorf("expected ID 'test-mem-1', got %q", mem.ID)
	}
	if mem.Content != "User prefers Go for backend" {
		t.Errorf("expected content 'User prefers Go for backend', got %q", mem.Content)
	}
}

// --------------------------------------------------------------------------
// Tool handler: memory_set (round-trip)
// --------------------------------------------------------------------------

func TestToolMemorySetAndSearch(t *testing.T) {
	s := helperServer(t)

	// Set a memory
	args, _ := json.Marshal(map[string]string{
		"content":  "The API server runs on port 8080",
		"scope":    "project",
		"metadata": `{"source":"test"}`,
	})
	params := CallToolParams{Name: "memory_set", Arguments: args}
	paramsJSON, _ := json.Marshal(params)
	output := captureResponse(func() {
		s.handleRequest(&JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      json.RawMessage(`"req-9"`),
			Method:  "tools/call",
			Params:  paramsJSON,
		})
	})

	var res JSONRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &res); err != nil {
		t.Fatalf("failed to parse memory_set response: %v\noutput: %s", err, output)
	}
	if res.Error != nil {
		t.Fatalf("unexpected error: %+v", res.Error)
	}
	result, ok := res.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result")
	}
	content, ok := result["content"].([]interface{})
	if !ok || len(content) == 0 {
		t.Fatal("expected content array")
	}
	text, _ := content[0].(map[string]interface{})["text"].(string)
	if !strings.Contains(text, "Successfully saved memory") {
		t.Errorf("expected success message, got %q", text)
	}

	// Search for the memory
	sargs, _ := json.Marshal(map[string]string{"query": "port 8080", "scope": "project", "limit": "5"})
	sparams := CallToolParams{Name: "memory_search", Arguments: sargs}
	sparamsJSON, _ := json.Marshal(sparams)
	soutput := captureResponse(func() {
		s.handleRequest(&JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      json.RawMessage(`"req-10"`),
			Method:  "tools/call",
			Params:  sparamsJSON,
		})
	})

	var sres JSONRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(soutput)), &sres); err != nil {
		t.Fatalf("failed to parse memory_search response: %v\noutput: %s", err, soutput)
	}
	if sres.Error != nil {
		t.Fatalf("unexpected error: %+v", sres.Error)
	}
}

// --------------------------------------------------------------------------
// Tool handler: memory_search with empty results
// --------------------------------------------------------------------------

func TestToolMemorySearchEmpty(t *testing.T) {
	s := helperServer(t)

	sargs, _ := json.Marshal(map[string]string{"query": "nonexistent topic", "limit": "3"})
	sparams := CallToolParams{Name: "memory_search", Arguments: sargs}
	sparamsJSON, _ := json.Marshal(sparams)
	output := captureResponse(func() {
		s.handleRequest(&JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      json.RawMessage(`"req-11"`),
			Method:  "tools/call",
			Params:  sparamsJSON,
		})
	})

	var res JSONRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &res); err != nil {
		t.Fatalf("failed to parse response: %v\noutput: %s", err, output)
	}
	if res.Error != nil {
		t.Fatalf("unexpected error: %+v", res.Error)
	}
	result, ok := res.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result")
	}
	content, ok := result["content"].([]interface{})
	if !ok || len(content) == 0 {
		t.Fatal("expected content array")
	}
	text, _ := content[0].(map[string]interface{})["text"].(string)
	if !strings.Contains(text, "No relevant memories found") {
		t.Errorf("expected 'No relevant memories found', got %q", text)
	}
}

// --------------------------------------------------------------------------
// Tool handler: memory_list (empty and with data)
// --------------------------------------------------------------------------

func TestToolMemoryListEmpty(t *testing.T) {
	s := helperServer(t)

	args, _ := json.Marshal(map[string]string{})
	params := CallToolParams{Name: "memory_list", Arguments: args}
	paramsJSON, _ := json.Marshal(params)
	output := captureResponse(func() {
		s.handleRequest(&JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      json.RawMessage(`"req-12"`),
			Method:  "tools/call",
			Params:  paramsJSON,
		})
	})

	var res JSONRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &res); err != nil {
		t.Fatalf("failed to parse response: %v\noutput: %s", err, output)
	}
	if res.Error != nil {
		t.Fatalf("unexpected error: %+v", res.Error)
	}
	result, ok := res.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result")
	}
	content, ok := result["content"].([]interface{})
	if !ok || len(content) == 0 {
		t.Fatal("expected content array")
	}
	text, _ := content[0].(map[string]interface{})["text"].(string)
	if !strings.Contains(text, "Memory store is empty") {
		t.Errorf("expected 'Memory store is empty', got %q", text)
	}
}

func TestToolMemoryListWithMemories(t *testing.T) {
	s := helperServer(t)

	// Seed two memories
	m1 := &db.Memory{
		ID:        "list-1",
		Content:   "Memory A",
		Scope:     "global",
		Embedding: []float32{1.0},
	}
	m2 := &db.Memory{
		ID:        "list-2",
		Content:   "Memory B",
		Scope:     "project",
		Embedding: []float32{2.0},
	}
	s.db.SaveMemory(m1)
	s.db.SaveMemory(m2)

	args, _ := json.Marshal(map[string]string{})
	params := CallToolParams{Name: "memory_list", Arguments: args}
	paramsJSON, _ := json.Marshal(params)
	output := captureResponse(func() {
		s.handleRequest(&JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      json.RawMessage(`"req-13"`),
			Method:  "tools/call",
			Params:  paramsJSON,
		})
	})

	var res JSONRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &res); err != nil {
		t.Fatalf("failed to parse response: %v\noutput: %s", err, output)
	}
	if res.Error != nil {
		t.Fatalf("unexpected error: %+v", res.Error)
	}

	result, ok := res.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result")
	}
	content, ok := result["content"].([]interface{})
	if !ok || len(content) == 0 {
		t.Fatal("expected content array")
	}
	text, _ := content[0].(map[string]interface{})["text"].(string)

	var mems []*db.Memory
	if err := json.Unmarshal([]byte(text), &mems); err != nil {
		t.Fatalf("failed to unmarshal memories: %v\ntext: %s", err, text)
	}
	if len(mems) != 2 {
		t.Errorf("expected 2 memories, got %d", len(mems))
	}
}

// --------------------------------------------------------------------------
// Response helpers (edge cases)
// --------------------------------------------------------------------------

func TestSendErrorResponse(t *testing.T) {
	s := helperServer(t)
	output := captureResponse(func() {
		s.sendError(json.RawMessage(`"err-1"`), -32000, "Server error")
	})

	var res JSONRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &res); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	if res.Error == nil {
		t.Fatal("expected error")
	}
	if res.Error.Code != -32000 {
		t.Errorf("expected code -32000, got %d", res.Error.Code)
	}
	if res.Error.Message != "Server error" {
		t.Errorf("expected message 'Server error', got %q", res.Error.Message)
	}
	if string(res.ID) != `"err-1"` {
		t.Errorf("expected id 'err-1', got %s", string(res.ID))
	}
}

func TestSendToolResponseError(t *testing.T) {
	s := helperServer(t)
	output := captureResponse(func() {
		s.sendToolResponse(json.RawMessage(`"e-1"`), "Something failed", true)
	})

	var res JSONRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &res); err != nil {
		t.Fatalf("failed to parse response: %v\noutput: %s", err, output)
	}
	result, ok := res.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result")
	}
	content, ok := result["content"].([]interface{})
	if !ok || len(content) == 0 {
		t.Fatal("expected content array")
	}
	text, _ := content[0].(map[string]interface{})["text"].(string)
	if !strings.HasPrefix(text, "[ERROR]") {
		t.Errorf("expected '[ERROR]' prefix, got %q", text)
	}
	if !strings.Contains(text, "Something failed") {
		t.Errorf("expected 'Something failed', got %q", text)
	}
}

// --------------------------------------------------------------------------
// HTTP sync endpoint helpers
// --------------------------------------------------------------------------

func helperAuthToken(t *testing.T, s *Server) string {
	t.Helper()
	token, err := s.jwts.GenerateToken("test", time.Hour)
	if err != nil {
		t.Fatalf("failed to generate test token: %v", err)
	}
	return token
}

// --------------------------------------------------------------------------
// Sync: unauthenticated requests return 401
// --------------------------------------------------------------------------

func TestSyncChangesUnauthenticated(t *testing.T) {
	s := helperServer(t)
	ts := httptest.NewServer(s.httpMux())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/api/sync/changes")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", res.StatusCode)
	}
}

func TestSyncApplyUnauthenticated(t *testing.T) {
	s := helperServer(t)
	ts := httptest.NewServer(s.httpMux())
	defer ts.Close()

	res, err := http.Post(ts.URL+"/api/sync/apply", "application/json", strings.NewReader(`{"memories":[]}`))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", res.StatusCode)
	}
}

// --------------------------------------------------------------------------
// Sync: pull with since filters correctly
// --------------------------------------------------------------------------

func TestSyncChangesSinceFilter(t *testing.T) {
	s := helperServer(t)
	token := helperAuthToken(t, s)

	old := &db.Memory{
		ID:        "sync-old",
		Content:   "old memory",
		Scope:     "global",
		Metadata:  map[string]string{},
		Embedding: []float32{0.1},
	}
	s.db.SaveMemory(old)

	got, _ := s.db.GetMemory("sync-old")
	cutoff := got.UpdatedAt
	time.Sleep(50 * time.Millisecond)

	new := &db.Memory{
		ID:        "sync-new",
		Content:   "new memory",
		Scope:     "global",
		Metadata:  map[string]string{},
		Embedding: []float32{0.2},
	}
	s.db.SaveMemory(new)

	ts := httptest.NewServer(s.httpMux())
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/sync/changes?since="+cutoff.UTC().Format(time.RFC3339Nano), nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Memories   []*db.Memory `json:"memories"`
		ServerTime string       `json:"server_time"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(body.Memories) != 1 {
		t.Fatalf("expected 1 memory after since cutoff, got %d", len(body.Memories))
	}
	if body.Memories[0].ID != "sync-new" {
		t.Errorf("expected memory ID 'sync-new', got %q", body.Memories[0].ID)
	}
	if body.ServerTime == "" {
		t.Error("expected non-empty server_time")
	}
}

// --------------------------------------------------------------------------
// Sync: apply skips older, overwrites newer
// --------------------------------------------------------------------------

func TestSyncApplyOlderSkippedNewerOverwrites(t *testing.T) {
	s := helperServer(t)
	token := helperAuthToken(t, s)

	existing := &db.Memory{
		ID:        "apply-1",
		Content:   "original content",
		Scope:     "global",
		Metadata:  map[string]string{},
		Embedding: []float32{0.1},
	}
	s.db.SaveMemory(existing)

	got, _ := s.db.GetMemory("apply-1")
	existingTime := got.UpdatedAt

	older := &db.Memory{
		ID:        "apply-1",
		Content:   "should be skipped",
		Scope:     "global",
		Metadata:  map[string]string{},
		Embedding: []float32{0.1},
		UpdatedAt: existingTime.Add(-1 * time.Hour),
		CreatedAt: existingTime.Add(-1 * time.Hour),
	}
	newer := &db.Memory{
		ID:        "apply-1",
		Content:   "should overwrite",
		Scope:     "global",
		Metadata:  map[string]string{},
		Embedding: []float32{0.1},
		UpdatedAt: existingTime.Add(1 * time.Hour),
		CreatedAt: existingTime,
	}
	fresh := &db.Memory{
		ID:        "apply-2",
		Content:   "brand new",
		Scope:     "global",
		Metadata:  map[string]string{},
		Embedding: []float32{0.3},
		UpdatedAt: time.Now().UTC(),
		CreatedAt: time.Now().UTC(),
	}

	payload, _ := json.Marshal(map[string][]*db.Memory{
		"memories": {older, newer, fresh},
	})

	ts := httptest.NewServer(s.httpMux())
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/sync/apply", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result struct {
		Applied int `json:"applied"`
		Skipped int `json:"skipped"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.Applied != 2 {
		t.Errorf("expected 2 applied (newer + fresh), got %d", result.Applied)
	}
	if result.Skipped != 1 {
		t.Errorf("expected 1 skipped (older), got %d", result.Skipped)
	}

	updated, _ := s.db.GetMemory("apply-1")
	if updated.Content != "should overwrite" {
		t.Errorf("expected content 'should overwrite', got %q", updated.Content)
	}
}

// --------------------------------------------------------------------------
// GET /api/get
// --------------------------------------------------------------------------

func TestApiGetUnauthenticated(t *testing.T) {
	s := helperServer(t)
	ts := httptest.NewServer(s.httpMux())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/api/get?id=anything")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", res.StatusCode)
	}
}

func TestApiGetMissingID(t *testing.T) {
	s := helperServer(t)
	token := helperAuthToken(t, s)
	ts := httptest.NewServer(s.httpMux())
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/get", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestApiGetNotFound(t *testing.T) {
	s := helperServer(t)
	token := helperAuthToken(t, s)
	ts := httptest.NewServer(s.httpMux())
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/get?id=nonexistent-id", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["error"] != "not found" {
		t.Errorf("expected error 'not found', got %q", body["error"])
	}
}

func TestApiGetSuccess(t *testing.T) {
	s := helperServer(t)
	token := helperAuthToken(t, s)

	m := &db.Memory{
		ID:        "get-test-1",
		Content:   "Test memory for GET endpoint",
		Scope:     "global",
		Metadata:  map[string]string{"source": "test"},
		Embedding: []float32{0.1, 0.2},
	}
	s.db.SaveMemory(m)

	ts := httptest.NewServer(s.httpMux())
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/get?id=get-test-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var got db.Memory
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.ID != "get-test-1" {
		t.Errorf("expected ID 'get-test-1', got %q", got.ID)
	}
	if got.Content != "Test memory for GET endpoint" {
		t.Errorf("expected content match, got %q", got.Content)
	}
}

// --------------------------------------------------------------------------
// DELETE /api/delete
// --------------------------------------------------------------------------

func TestApiDeleteUnauthenticated(t *testing.T) {
	s := helperServer(t)
	ts := httptest.NewServer(s.httpMux())
	defer ts.Close()

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/delete?id=anything", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", res.StatusCode)
	}
}

func TestApiDeleteMissingID(t *testing.T) {
	s := helperServer(t)
	token := helperAuthToken(t, s)
	ts := httptest.NewServer(s.httpMux())
	defer ts.Close()

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/delete", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestApiDeleteNotFound(t *testing.T) {
	s := helperServer(t)
	token := helperAuthToken(t, s)
	ts := httptest.NewServer(s.httpMux())
	defer ts.Close()

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/delete?id=nonexistent-id", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestApiDeleteSuccess(t *testing.T) {
	s := helperServer(t)
	token := helperAuthToken(t, s)

	m := &db.Memory{
		ID:        "del-test-1",
		Content:   "To be deleted",
		Scope:     "global",
		Metadata:  map[string]string{},
		Embedding: []float32{0.1},
	}
	s.db.SaveMemory(m)

	ts := httptest.NewServer(s.httpMux())
	defer ts.Close()

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/delete?id=del-test-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]bool
	json.NewDecoder(resp.Body).Decode(&body)
	if !body["deleted"] {
		t.Error("expected deleted: true")
	}

	got, _ := s.db.GetMemory("del-test-1")
	if got != nil {
		t.Error("expected memory to be deleted from database")
	}
}

func TestApiDeleteViaPost(t *testing.T) {
	s := helperServer(t)
	token := helperAuthToken(t, s)

	m := &db.Memory{
		ID:        "del-post-1",
		Content:   "To be deleted via POST",
		Scope:     "global",
		Metadata:  map[string]string{},
		Embedding: []float32{0.1},
	}
	s.db.SaveMemory(m)

	ts := httptest.NewServer(s.httpMux())
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/delete?id=del-post-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	got, _ := s.db.GetMemory("del-post-1")
	if got != nil {
		t.Error("expected memory to be deleted from database")
	}
}

// --------------------------------------------------------------------------
// GET /api/rules
// --------------------------------------------------------------------------

func TestApiRulesUnauthenticated(t *testing.T) {
	s := helperServer(t)
	ts := httptest.NewServer(s.httpMux())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/api/rules")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", res.StatusCode)
	}
}

func TestApiRulesEmpty(t *testing.T) {
	s := helperServer(t)
	token := helperAuthToken(t, s)
	ts := httptest.NewServer(s.httpMux())
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/rules", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Rules []db.Rule `json:"rules"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(body.Rules))
	}
}

func TestApiRulesWithData(t *testing.T) {
	s := helperServer(t)
	token := helperAuthToken(t, s)

	r := &db.Rule{
		ID:       "rule-1",
		Content:  "Always use tabs for indentation",
		Scope:    "global",
		Metadata: map[string]string{"source": "test"},
	}
	s.db.SaveRule(r)

	ts := httptest.NewServer(s.httpMux())
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/rules", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Rules []db.Rule `json:"rules"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(body.Rules))
	}
	if body.Rules[0].ID != "rule-1" {
		t.Errorf("expected rule ID 'rule-1', got %q", body.Rules[0].ID)
	}
	if body.Rules[0].Content != "Always use tabs for indentation" {
		t.Errorf("expected rule content match, got %q", body.Rules[0].Content)
	}
}

// --------------------------------------------------------------------------
// Web Console: embedded dashboard
// --------------------------------------------------------------------------

func TestWebConsoleReturnsHTML(t *testing.T) {
	s := helperServer(t)
	ts := httptest.NewServer(s.httpMux())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", res.StatusCode)
	}

	contentType := res.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("expected text/html content type, got %q", contentType)
	}

	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "Symaira Memory Console") {
		t.Error("expected HTML to contain 'Symaira Memory Console'")
	}
}

func TestWebConsoleStaticAssets(t *testing.T) {
	s := helperServer(t)
	ts := httptest.NewServer(s.httpMux())
	defer ts.Close()

	assets := []struct {
		path        string
		contentType string
	}{
		{"/style.css", "text/css"},
		{"/app.js", "javascript"},
	}

	for _, asset := range assets {
		res, err := http.Get(ts.URL + asset.path)
		if err != nil {
			t.Errorf("request for %s failed: %v", asset.path, err)
			continue
		}
		res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Errorf("expected 200 for %s, got %d", asset.path, res.StatusCode)
		}

		ct := res.Header.Get("Content-Type")
		if !strings.Contains(ct, asset.contentType) {
			t.Errorf("expected %s content type for %s, got %q", asset.contentType, asset.path, ct)
		}
	}
}

func TestWebConsoleDoesNotShadowAPIRoutes(t *testing.T) {
	s := helperServer(t)
	token := helperAuthToken(t, s)
	ts := httptest.NewServer(s.httpMux())
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for /api/status, got %d", resp.StatusCode)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "healthy" {
		t.Errorf("expected status 'healthy', got %q", body["status"])
	}
}

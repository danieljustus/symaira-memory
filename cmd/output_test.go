package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/danieljustus/symaira-memory/internal/db"
)

func sampleMemory() *db.Memory {
	return &db.Memory{
		ID:        "abc12345-def6-7890-abcd-ef1234567890",
		Content:   "Alice prefers dark mode in all applications.",
		Scope:     "global",
		Metadata:  map[string]string{"source": "chat"},
		Embedding: []float32{0.1, 0.2, 0.3},
		CreatedAt: time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		Entities:  []string{"alice"},
		CreatedBy: "test-agent",
	}
}

// --------------------------------------------------------------------------
// truncateString
// --------------------------------------------------------------------------

func TestTruncateStringShort(t *testing.T) {
	got := truncateString(10, "hello")
	if got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestTruncateStringLong(t *testing.T) {
	got := truncateString(5, "hello world")
	if got != "hello..." {
		t.Errorf("expected 'hello...', got %q", got)
	}
}

func TestTruncateStringExact(t *testing.T) {
	got := truncateString(5, "hello")
	if got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

// --------------------------------------------------------------------------
// FormatJSON
// --------------------------------------------------------------------------

func TestFormatJSONSingleMemory(t *testing.T) {
	var buf bytes.Buffer
	f := &OutputFormatter{Format: "json", Writer: &buf}

	err := f.FormatJSON(sampleMemory())
	if err != nil {
		t.Fatalf("FormatJSON error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if parsed["id"] != "abc12345-def6-7890-abcd-ef1234567890" {
		t.Errorf("expected id in JSON output, got %v", parsed["id"])
	}
}

func TestFormatJSONEmptyList(t *testing.T) {
	var buf bytes.Buffer
	f := &OutputFormatter{Format: "json", Writer: &buf}

	err := f.FormatJSON([]db.Memory{})
	if err != nil {
		t.Fatalf("FormatJSON error: %v", err)
	}

	output := strings.TrimSpace(buf.String())
	if output != "[]" {
		t.Errorf("expected '[]', got %q", output)
	}
}

// --------------------------------------------------------------------------
// FormatText - list template
// --------------------------------------------------------------------------

func TestFormatTextListEmpty(t *testing.T) {
	var buf bytes.Buffer
	f := &OutputFormatter{Format: "text", Writer: &buf}

	err := f.FormatText([]db.Memory{}, defaultTextTemplates["list"])
	if err != nil {
		t.Fatalf("FormatText error: %v", err)
	}

	if !strings.Contains(buf.String(), "No memories found") {
		t.Errorf("expected 'No memories found', got %q", buf.String())
	}
}

func TestFormatTextListWithEntries(t *testing.T) {
	var buf bytes.Buffer
	f := &OutputFormatter{Format: "text", Writer: &buf}

	mems := []db.Memory{*sampleMemory()}
	err := f.FormatText(mems, defaultTextTemplates["list"])
	if err != nil {
		t.Fatalf("FormatText error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "abc12345") {
		t.Errorf("expected truncated ID in output, got %q", output)
	}
	if !strings.Contains(output, "global") {
		t.Errorf("expected scope in output, got %q", output)
	}
	if !strings.Contains(output, "Alice prefers dark mode") {
		t.Errorf("expected content in output, got %q", output)
	}
}

// --------------------------------------------------------------------------
// FormatText - search template
// --------------------------------------------------------------------------

func TestFormatTextSearchEmpty(t *testing.T) {
	var buf bytes.Buffer
	f := &OutputFormatter{Format: "text", Writer: &buf}

	err := f.FormatText([]db.SearchResult{}, defaultTextTemplates["search"])
	if err != nil {
		t.Fatalf("FormatText error: %v", err)
	}

	if !strings.Contains(buf.String(), "No relevant memories found") {
		t.Errorf("expected 'No relevant memories found', got %q", buf.String())
	}
}

func TestFormatTextSearchWithResults(t *testing.T) {
	var buf bytes.Buffer
	f := &OutputFormatter{Format: "text", Writer: &buf}

	results := []db.SearchResult{
		{Memory: sampleMemory(), Score: 0.9523},
	}
	err := f.FormatText(results, defaultTextTemplates["search"])
	if err != nil {
		t.Fatalf("FormatText error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "0.9523") {
		t.Errorf("expected score in output, got %q", output)
	}
	if !strings.Contains(output, "abc12345") {
		t.Errorf("expected truncated ID in output, got %q", output)
	}
}

// --------------------------------------------------------------------------
// FormatText - get template
// --------------------------------------------------------------------------

func TestFormatTextGetSingleMemory(t *testing.T) {
	var buf bytes.Buffer
	f := &OutputFormatter{Format: "text", Writer: &buf}

	err := f.FormatText(sampleMemory(), defaultTextTemplates["get"])
	if err != nil {
		t.Fatalf("FormatText error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "abc12345-def6-7890-abcd-ef1234567890") {
		t.Errorf("expected full ID in output, got %q", output)
	}
	if !strings.Contains(output, "global") {
		t.Errorf("expected scope in output, got %q", output)
	}
	if !strings.Contains(output, "Alice prefers dark mode") {
		t.Errorf("expected full content in output, got %q", output)
	}
	if !strings.Contains(output, "2026-01-15") {
		t.Errorf("expected formatted date in output, got %q", output)
	}
	if !strings.Contains(output, "alice") {
		t.Errorf("expected entity in output, got %q", output)
	}
}

// --------------------------------------------------------------------------
// Output dispatcher
// --------------------------------------------------------------------------

func TestOutputDispatchesJSON(t *testing.T) {
	var buf bytes.Buffer
	f := &OutputFormatter{Format: "json", Writer: &buf}

	err := f.Output(sampleMemory(), "get")
	if err != nil {
		t.Fatalf("Output error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("expected valid JSON, got: %v", err)
	}
}

func TestOutputDispatchesText(t *testing.T) {
	var buf bytes.Buffer
	f := &OutputFormatter{Format: "text", Writer: &buf}

	err := f.Output(sampleMemory(), "get")
	if err != nil {
		t.Fatalf("Output error: %v", err)
	}

	if !strings.Contains(buf.String(), "ID:") {
		t.Errorf("expected text format with 'ID:', got %q", buf.String())
	}
}

func TestOutputUnknownTemplate(t *testing.T) {
	var buf bytes.Buffer
	f := &OutputFormatter{Format: "text", Writer: &buf}

	err := f.Output(sampleMemory(), "nonexistent")
	if err == nil {
		t.Error("expected error for unknown template name")
	}
}

// --------------------------------------------------------------------------
// Flag registration
// --------------------------------------------------------------------------

func TestOutputFlagRegisteredOnRoot(t *testing.T) {
	flag := rootCmd.PersistentFlags().Lookup("output")
	if flag == nil {
		t.Fatal("expected 'output' persistent flag on root command")
	}
	if flag.DefValue != "table" {
		t.Errorf("expected default 'table', got %q", flag.DefValue)
	}
	if flag.Shorthand != "o" {
		t.Errorf("expected shorthand 'o', got %q", flag.Shorthand)
	}
}

func TestFormatFlagIsHiddenAlias(t *testing.T) {
	flag := rootCmd.PersistentFlags().Lookup("format")
	if flag == nil {
		t.Fatal("expected hidden 'format' alias on root command")
	}
	if flag.DefValue != "table" {
		t.Errorf("expected default 'table', got %q", flag.DefValue)
	}
	if !flag.Hidden {
		t.Error("expected 'format' flag to be hidden")
	}
}

func TestNoLocalFormatFlags(t *testing.T) {
	if listCmd.Flags().Lookup("format") != nil {
		t.Error("expected no local 'format' flag on list command")
	}
	if searchCmd.Flags().Lookup("format") != nil {
		t.Error("expected no local 'format' flag on search command")
	}
	if getCmd.Flags().Lookup("format") != nil {
		t.Error("expected no local 'format' flag on get command")
	}
}

// --------------------------------------------------------------------------
// Embedding inclusion/omission
// --------------------------------------------------------------------------

func TestSearchCommandHasIncludeEmbeddingFlag(t *testing.T) {
	flag := searchCmd.Flags().Lookup("include-embedding")
	if flag == nil {
		t.Fatal("expected 'include-embedding' flag on search command")
	}
	if flag.DefValue != "false" {
		t.Errorf("expected default 'false', got %q", flag.DefValue)
	}
}

func TestGetCommandHasIncludeEmbeddingFlag(t *testing.T) {
	flag := getCmd.Flags().Lookup("include-embedding")
	if flag == nil {
		t.Fatal("expected 'include-embedding' flag on get command")
	}
	if flag.DefValue != "false" {
		t.Errorf("expected default 'false', got %q", flag.DefValue)
	}
}

func TestFormatJSONOmitsEmbeddingByDefault(t *testing.T) {
	var buf bytes.Buffer
	f := &OutputFormatter{Format: "json", Writer: &buf, IncludeEmbedding: false}

	err := f.FormatJSON(sampleMemory())
	if err != nil {
		t.Fatalf("FormatJSON error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if _, exists := parsed["embedding"]; exists {
		t.Error("expected embedding to be omitted from JSON output by default")
	}
	if parsed["id"] != "abc12345-def6-7890-abcd-ef1234567890" {
		t.Errorf("expected id in JSON output, got %v", parsed["id"])
	}
}

func TestFormatJSONIncludesEmbeddingWhenRequested(t *testing.T) {
	var buf bytes.Buffer
	f := &OutputFormatter{Format: "json", Writer: &buf, IncludeEmbedding: true}

	err := f.FormatJSON(sampleMemory())
	if err != nil {
		t.Fatalf("FormatJSON error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	emb, exists := parsed["embedding"]
	if !exists {
		t.Error("expected embedding to be present in JSON output when IncludeEmbedding=true")
	}
	embSlice, ok := emb.([]interface{})
	if !ok || len(embSlice) != 3 {
		t.Errorf("expected embedding to be array of length 3, got %v", emb)
	}
}

func TestFormatJSONSearchResultOmitsEmbeddingByDefault(t *testing.T) {
	var buf bytes.Buffer
	f := &OutputFormatter{Format: "json", Writer: &buf, IncludeEmbedding: false}

	results := []db.SearchResult{
		{Memory: sampleMemory(), Score: 0.95},
	}
	err := f.FormatJSON(results)
	if err != nil {
		t.Fatalf("FormatJSON error: %v", err)
	}

	var parsed []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 result, got %d", len(parsed))
	}
	mem, ok := parsed[0]["memory"].(map[string]interface{})
	if !ok {
		t.Fatal("expected memory object in result")
	}
	if _, exists := mem["embedding"]; exists {
		t.Error("expected embedding to be omitted from SearchResult JSON by default")
	}
	if parsed[0]["similarity_score"] == nil {
		t.Error("expected similarity_score in result")
	}
}

func TestFormatJSONSearchResultIncludesEmbeddingWhenRequested(t *testing.T) {
	var buf bytes.Buffer
	f := &OutputFormatter{Format: "json", Writer: &buf, IncludeEmbedding: true}

	results := []db.SearchResult{
		{Memory: sampleMemory(), Score: 0.95},
	}
	err := f.FormatJSON(results)
	if err != nil {
		t.Fatalf("FormatJSON error: %v", err)
	}

	var parsed []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	mem, ok := parsed[0]["memory"].(map[string]interface{})
	if !ok {
		t.Fatal("expected memory object in result")
	}
	if _, exists := mem["embedding"]; !exists {
		t.Error("expected embedding to be present in SearchResult JSON when IncludeEmbedding=true")
	}
}

func TestStripEmbeddingDoesNotMutateOriginal(t *testing.T) {
	orig := sampleMemory()
	origLen := len(orig.Embedding)

	_ = stripEmbedding(orig)

	if len(orig.Embedding) != origLen {
		t.Errorf("stripEmbedding mutated original: expected %d elements, got %d", origLen, len(orig.Embedding))
	}
}

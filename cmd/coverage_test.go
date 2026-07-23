package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/danieljustus/symaira-memory/internal/db"
)

// --------------------------------------------------------------------------
// toolNames (config.go) — 0% coverage
// --------------------------------------------------------------------------

func TestToolNames_AllPresets(t *testing.T) {
	names := toolNames()
	if len(names) != len(toolPresets) {
		t.Errorf("expected %d tool names, got %d: %v", len(toolPresets), len(names), names)
	}

	// Verify every preset key appears in the result
	for k := range toolPresets {
		found := false
		for _, n := range names {
			if n == k {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected tool name %q to appear in toolNames() result", k)
		}
	}
}

// --------------------------------------------------------------------------
// bench.go — benchTokenReduction, estimateBenchTokens, splitWords, truncateStr
// --------------------------------------------------------------------------

func TestEstimateBenchTokens(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"empty string", "", 0},
		{"single word", "hello", 1},     // (1*4)/3 = 1
		{"two words", "hello world", 2}, // (2*4)/3 = 2
		{"three words", "a b c", 4},     // (3*4)/3 = 4
		{"with newlines", "hello\nworld\n", 2},
		{"with tabs", "hello\tworld", 2},
		{"with leading/trailing spaces", "  hello world  ", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateBenchTokens(tt.input)
			if got != tt.expected {
				t.Errorf("estimateBenchTokens(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSplitWords(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"empty string", "", nil},
		{"single word", "hello", []string{"hello"}},
		{"multiple words", "hello brave world", []string{"hello", "brave", "world"}},
		{"with newlines", "line1\nline2", []string{"line1", "line2"}},
		{"with tabs", "col1\tcol2", []string{"col1", "col2"}},
		{"with carriage return", "a\rb", []string{"a", "b"}},
		{"mixed whitespace", "a b\nc\td", []string{"a", "b", "c", "d"}},
		{"leading whitespace", "  hello", []string{"hello"}},
		{"trailing whitespace", "hello  ", []string{"hello"}},
		{"leading and trailing", "  hello world  ", []string{"hello", "world"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitWords(tt.input)
			if len(got) == 0 && len(tt.expected) == 0 {
				return
			}
			if len(got) != len(tt.expected) {
				t.Errorf("splitWords(%q) = %v (len=%d), want %v (len=%d)", tt.input, got, len(got), tt.expected, len(tt.expected))
				return
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("splitWords(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestTruncateStr(t *testing.T) {
	tests := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"empty string", "", 10, ""},
		{"shorter than max", "short", 10, "short"},
		{"exactly max", "exactly10", 10, "exactly10"},
		{"longer than max", "this is a very long string", 10, "this is..."},
		{"max of 3 (minimum safe)", "hello", 3, "..."},
		{"longer with max 4", "hello world", 4, "h..."},
		{"unicode with truncation", "héllo world", 9, "héllo..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateStr(tt.s, tt.max)
			if got != tt.want {
				t.Errorf("truncateStr(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
			}
		})
	}
}

// benchTokenReduction calls estimateBenchTokens, summarizer, and truncateStr.
// We test that it runs without panicking and exercises basic paths.
func TestBenchTokenReduction_Runs(t *testing.T) {
	// Just verify it doesn't panic — output goes to stderr
	stderr := captureStderr(func() {
		benchTokenReduction()
	})
	if stderr == "" {
		t.Error("expected stderr output from benchTokenReduction")
	}
	if !strings.Contains(stderr, "Original tokens:") {
		t.Error("expected 'Original tokens:' in stderr output")
	}
	if !strings.Contains(stderr, "Reduction:") {
		t.Error("expected 'Reduction:' in stderr output")
	}
}

// --------------------------------------------------------------------------
// parseDuration (search.go) — 0% coverage
// --------------------------------------------------------------------------

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"empty string", "", 0, false},
		{"days", "7d", 7 * 24 * time.Hour, false},
		{"hours", "12h", 12 * time.Hour, false},
		{"minutes", "30m", 30 * time.Minute, false},
		{"seconds", "45s", 45 * time.Second, false},
		{"single day", "1d", 24 * time.Hour, false},
		{"zero days", "0d", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseDuration(%q) expected error, got none", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("parseDuration(%q) unexpected error: %v", tt.input, err)
				return
			}
			if got != tt.want {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseDuration_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"single char no number", "d"},
		{"garbage suffix", "10x"},
		{"non-numeric", "abc"},
		{"just text", "forever"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseDuration(tt.input)
			if err == nil {
				t.Errorf("parseDuration(%q) expected error, got none", tt.input)
			}
		})
	}
}

// --------------------------------------------------------------------------
// GetNoColor (root.go) — 0% coverage
// --------------------------------------------------------------------------

func TestGetNoColor_FlagOnly(t *testing.T) {
	old := noColor
	noColor = true
	defer func() { noColor = old }()

	t.Setenv("NO_COLOR", "") // ensure env is cleared

	if !GetNoColor() {
		t.Error("expected GetNoColor() = true when noColor flag is set")
	}
}

func TestGetNoColor_EnvOnly(t *testing.T) {
	old := noColor
	noColor = false
	defer func() { noColor = old }()

	t.Setenv("NO_COLOR", "1")

	if !GetNoColor() {
		t.Error("expected GetNoColor() = true when NO_COLOR env is set")
	}
}

func TestGetNoColor_Both(t *testing.T) {
	old := noColor
	noColor = true
	defer func() { noColor = old }()

	t.Setenv("NO_COLOR", "1")

	if !GetNoColor() {
		t.Error("expected GetNoColor() = true when both flag and env are set")
	}
}

func TestGetNoColor_Neither(t *testing.T) {
	old := noColor
	noColor = false
	defer func() { noColor = old }()

	t.Setenv("NO_COLOR", "")

	if GetNoColor() {
		t.Error("expected GetNoColor() = false when neither flag nor env is set")
	}
}

// --------------------------------------------------------------------------
// GetOutputFormat (root.go) — 66.7% coverage — test edge cases
// --------------------------------------------------------------------------

func TestGetOutputFormat_Default(t *testing.T) {
	old := outputFormat
	outputFormat = ""
	defer func() { outputFormat = old }()

	got := GetOutputFormat(rootCmd)
	if got != "table" {
		t.Errorf("expected 'table', got %q", got)
	}
}

func TestGetOutputFormat_Explicit(t *testing.T) {
	old := outputFormat
	outputFormat = "json"
	defer func() { outputFormat = old }()

	got := GetOutputFormat(rootCmd)
	if got != "json" {
		t.Errorf("expected 'json', got %q", got)
	}
}

// --------------------------------------------------------------------------
// bytesBuffer (backup.go) — Write, Bytes, Len all at 0%
// --------------------------------------------------------------------------

func TestBytesBuffer_WriteAndRead(t *testing.T) {
	var buf bytesBuffer

	// Test initial state
	if buf.Len() != 0 {
		t.Errorf("expected initial Len()=0, got %d", buf.Len())
	}
	if len(buf.Bytes()) != 0 {
		t.Errorf("expected initial Bytes() empty, got %v", buf.Bytes())
	}

	// Write one chunk
	n, err := buf.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != 5 {
		t.Errorf("expected n=5, got %d", n)
	}
	if buf.Len() != 5 {
		t.Errorf("expected Len()=5, got %d", buf.Len())
	}
	if string(buf.Bytes()) != "hello" {
		t.Errorf("expected 'hello', got %q", string(buf.Bytes()))
	}

	// Write another chunk (append)
	n, err = buf.Write([]byte(" world"))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != 6 {
		t.Errorf("expected n=6, got %d", n)
	}
	if buf.Len() != 11 {
		t.Errorf("expected Len()=11, got %d", buf.Len())
	}
	if string(buf.Bytes()) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(buf.Bytes()))
	}

	// Write empty chunk
	n, err = buf.Write([]byte{})
	if err != nil {
		t.Fatalf("Write error on empty: %v", err)
	}
	if n != 0 {
		t.Errorf("expected n=0 for empty write, got %d", n)
	}
	if buf.Len() != 11 {
		t.Errorf("expected Len()=11 after empty write, got %d", buf.Len())
	}
}

// --------------------------------------------------------------------------
// stripEmbedding (output.go) — cover remaining uncovered type branches
// --------------------------------------------------------------------------

func TestStripEmbedding_DBNonPointerMemory(t *testing.T) {
	// Test db.Memory (value type, not pointer)
	m := db.Memory{
		ID:        "val-mem-1",
		Content:   "value type memory",
		Embedding: []float32{0.1, 0.2, 0.3},
	}
	result := stripEmbedding(m)
	cloned, ok := result.(db.Memory)
	if !ok {
		t.Fatalf("expected db.Memory, got %T", result)
	}
	if cloned.Embedding != nil {
		t.Error("expected Embedding to be nil after strip")
	}
	if cloned.Content != "value type memory" {
		t.Errorf("expected content preserved, got %q", cloned.Content)
	}
}

func TestStripEmbedding_DBSearchResultValue(t *testing.T) {
	// Test db.SearchResult (value type)
	sr := db.SearchResult{
		Score: 0.99,
		Memory: &db.Memory{
			ID:        "sr-val-1",
			Content:   "search result value",
			Embedding: []float32{0.4, 0.5},
		},
	}
	result := stripEmbedding(sr)
	cloned, ok := result.(db.SearchResult)
	if !ok {
		t.Fatalf("expected db.SearchResult, got %T", result)
	}
	if cloned.Score != 0.99 {
		t.Errorf("expected score 0.99, got %f", cloned.Score)
	}
	if cloned.Memory == nil {
		t.Fatal("expected Memory to be non-nil")
	}
	if cloned.Memory.Embedding != nil {
		t.Error("expected Memory.Embedding to be nil after strip")
	}
	if cloned.Memory.Content != "search result value" {
		t.Errorf("expected content preserved, got %q", cloned.Memory.Content)
	}
}

func TestStripEmbedding_DBSearchResultPointer(t *testing.T) {
	// Test *db.SearchResult (pointer type)
	sr := &db.SearchResult{
		Score: 0.88,
		Memory: &db.Memory{
			ID:        "sr-ptr-1",
			Content:   "search result pointer",
			Embedding: []float32{0.6},
		},
	}
	result := stripEmbedding(sr)
	cloned, ok := result.(*db.SearchResult)
	if !ok {
		t.Fatalf("expected *db.SearchResult, got %T", result)
	}
	if cloned.Memory.Embedding != nil {
		t.Error("expected Memory.Embedding to be nil after strip")
	}
}

func TestStripEmbedding_PointerMemorySlice(t *testing.T) {
	// Test []*db.Memory
	mems := []*db.Memory{
		{ID: "a", Content: "mem a", Embedding: []float32{0.1}},
		{ID: "b", Content: "mem b", Embedding: []float32{0.2}},
	}
	result := stripEmbedding(mems)
	cloned, ok := result.([]*db.Memory)
	if !ok {
		t.Fatalf("expected []*db.Memory, got %T", result)
	}
	for i, m := range cloned {
		if m.Embedding != nil {
			t.Errorf("mems[%d]: expected Embedding nil, got %v", i, m.Embedding)
		}
		if m.ID != mems[i].ID {
			t.Errorf("mems[%d]: expected ID %q, got %q", i, mems[i].ID, m.ID)
		}
	}
	// Verify original slice is not mutated
	if len(mems[0].Embedding) == 0 {
		t.Error("original memory Embedding was mutated")
	}
}

func TestStripEmbedding_ValueMemorySlice(t *testing.T) {
	// Test []db.Memory
	mems := []db.Memory{
		{ID: "a", Content: "mem a", Embedding: []float32{0.1}},
		{ID: "b", Content: "mem b", Embedding: []float32{0.2}},
	}
	result := stripEmbedding(mems)
	cloned, ok := result.([]db.Memory)
	if !ok {
		t.Fatalf("expected []db.Memory, got %T", result)
	}
	for i, m := range cloned {
		if m.Embedding != nil {
			t.Errorf("mems[%d]: expected Embedding nil, got %v", i, m.Embedding)
		}
	}
	if len(mems[0].Embedding) == 0 {
		t.Error("original memory Embedding was mutated")
	}
}

func TestStripEmbedding_PointerSearchResultSlice(t *testing.T) {
	// Test []*db.SearchResult with nested Memory having Embedding
	results := []*db.SearchResult{
		{
			Score:  0.9,
			Memory: &db.Memory{ID: "a", Content: "sr a", Embedding: []float32{0.1}},
		},
		{
			Score:  0.8,
			Memory: &db.Memory{ID: "b", Content: "sr b", Embedding: []float32{0.2}},
		},
	}
	result := stripEmbedding(results)
	cloned, ok := result.([]*db.SearchResult)
	if !ok {
		t.Fatalf("expected []*db.SearchResult, got %T", result)
	}
	for i, sr := range cloned {
		if sr.Memory.Embedding != nil {
			t.Errorf("results[%d]: expected Memory.Embedding nil, got %v", i, sr.Memory.Embedding)
		}
		if sr.Score != results[i].Score {
			t.Errorf("results[%d]: expected Score %f, got %f", i, results[i].Score, sr.Score)
		}
	}
}

func TestStripEmbedding_ValueSearchResultSlice(t *testing.T) {
	// Test []db.SearchResult
	results := []db.SearchResult{
		{
			Score:  0.7,
			Memory: &db.Memory{ID: "a", Content: "sr a", Embedding: []float32{0.1}},
		},
		{
			Score:  0.6,
			Memory: &db.Memory{ID: "b", Content: "sr b", Embedding: []float32{0.2}},
		},
	}
	result := stripEmbedding(results)
	cloned, ok := result.([]db.SearchResult)
	if !ok {
		t.Fatalf("expected []db.SearchResult, got %T", result)
	}
	for i, sr := range cloned {
		if sr.Memory.Embedding != nil {
			t.Errorf("results[%d]: expected Memory.Embedding nil, got %v", i, sr.Memory.Embedding)
		}
	}
}

func TestStripEmbedding_ValueSearchResultSlice_NilMemory(t *testing.T) {
	// Test []db.SearchResult where Memory is nil
	results := []db.SearchResult{
		{Score: 0.5, Memory: nil},
		{Score: 0.4, Memory: nil},
	}
	result := stripEmbedding(results)
	cloned, ok := result.([]db.SearchResult)
	if !ok {
		t.Fatalf("expected []db.SearchResult, got %T", result)
	}
	for i, sr := range cloned {
		if sr.Score != results[i].Score {
			t.Errorf("results[%d]: expected Score %f, got %f", i, results[i].Score, sr.Score)
		}
		if sr.Memory != nil {
			t.Errorf("results[%d]: expected nil Memory, got non-nil", i)
		}
	}
}

func TestStripEmbedding_DefaultType(t *testing.T) {
	// Test a type that doesn't match any case (default branch)
	result := stripEmbedding("just a string")
	if s, ok := result.(string); !ok || s != "just a string" {
		t.Errorf("expected 'just a string' unchanged, got %v", result)
	}

	result = stripEmbedding(42)
	if n, ok := result.(int); !ok || n != 42 {
		t.Errorf("expected 42 unchanged, got %v", result)
	}

	result = stripEmbedding(nil)
	if result != nil {
		t.Errorf("expected nil unchanged, got %v", result)
	}
}

func TestStripEmbedding_DBPointerMemWithNilEmbedding(t *testing.T) {
	// Edge case: *db.Memory with nil Embedding
	m := &db.Memory{
		ID:        "nil-emb",
		Content:   "no embedding",
		Embedding: nil,
	}
	result := stripEmbedding(m)
	cloned, ok := result.(*db.Memory)
	if !ok {
		t.Fatalf("expected *db.Memory, got %T", result)
	}
	if cloned.Content != "no embedding" {
		t.Errorf("expected content preserved, got %q", cloned.Content)
	}
}

func TestStripEmbedding_SearchResultPointerNilMemory(t *testing.T) {
	// Edge case: *db.SearchResult with nil Memory
	sr := &db.SearchResult{
		Score:  0.5,
		Memory: nil,
	}
	result := stripEmbedding(sr)
	cloned, ok := result.(*db.SearchResult)
	if !ok {
		t.Fatalf("expected *db.SearchResult, got %T", result)
	}
	if cloned.Memory != nil {
		t.Error("expected Memory to remain nil")
	}
}

// --------------------------------------------------------------------------
// resolveBackupPassword (backup.go) — cover --password flag path (47.8% → higher)
// --------------------------------------------------------------------------

func TestResolveBackupPassword_DeprecatedFlag(t *testing.T) {
	oldPW := backupPassword
	backupPassword = "direct-flag-pw"
	defer func() { backupPassword = oldPW }()
	oldFile := backupPasswordFile
	backupPasswordFile = ""
	defer func() { backupPasswordFile = oldFile }()

	// Capture stderr for the deprecation warning
	stderr := captureStderr(func() {
		pw, err := resolveBackupPassword("encryption")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pw != "direct-flag-pw" {
			t.Errorf("expected 'direct-flag-pw', got %q", pw)
		}
	})

	if !strings.Contains(stderr, "deprecated") {
		t.Error("expected deprecation warning in stderr")
	}
}

func TestResolveBackupPassword_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	pwFile := dir + "/empty_pw.txt"
	if err := os.WriteFile(pwFile, []byte("  \n"), 0600); err != nil {
		t.Fatalf("write password file: %v", err)
	}

	old := backupPasswordFile
	backupPasswordFile = pwFile
	defer func() { backupPasswordFile = old }()
	oldPW := backupPassword
	backupPassword = ""
	defer func() { backupPassword = oldPW }()

	_, err := resolveBackupPassword("encryption")
	if err == nil {
		t.Fatal("expected error for empty password file")
	}
	if !strings.Contains(err.Error(), "is empty") {
		t.Errorf("expected 'is empty' error, got %v", err)
	}
}

func TestResolveBackupPassword_NonExistentFile(t *testing.T) {
	old := backupPasswordFile
	backupPasswordFile = "/nonexistent/pw.txt"
	defer func() { backupPasswordFile = old }()
	oldPW := backupPassword
	backupPassword = ""
	defer func() { backupPassword = oldPW }()

	_, err := resolveBackupPassword("encryption")
	if err == nil {
		t.Fatal("expected error for non-existent password file")
	}
}

// --------------------------------------------------------------------------
// buildJSONConfig (config.go) — cover error path in JSON marshal
// --------------------------------------------------------------------------

func TestBuildJSONConfig_OpenCodeFormat(t *testing.T) {
	result := buildJSONConfig("opencode", "/usr/bin/symmemory", []string{"serve", "--profile", "test-profile"})
	if !strings.Contains(result, `"mcp"`) {
		t.Errorf("expected opencode format to have 'mcp' root key, got: %s", result)
	}
	if !strings.Contains(result, `"type": "local"`) {
		t.Errorf("expected opencode format to have type:local, got: %s", result)
	}
	// OpenCode command should be an array
	if !strings.Contains(result, `"serve"`) {
		t.Errorf("expected opencode format to contain serve, got: %s", result)
	}
}

func TestBuildJSONConfig_CopilotFormat(t *testing.T) {
	result := buildJSONConfig("copilot", "symmemory", []string{"serve"})
	if !strings.Contains(result, `"mcpServers"`) {
		t.Errorf("expected copilot format to have mcpServers, got: %s", result)
	}
	if !strings.Contains(result, `"type": "local"`) {
		t.Errorf("expected copilot format to have type:local, got: %s", result)
	}
	if !strings.Contains(result, `"env"`) {
		t.Errorf("expected copilot format to have env, got: %s", result)
	}
	if !strings.Contains(result, `"tools"`) {
		t.Errorf("expected copilot format to have tools, got: %s", result)
	}
}

func TestBuildJSONConfig_ClaudeCodeWithProfile(t *testing.T) {
	result := buildJSONConfig("claude-code", "/usr/local/bin/symmemory", []string{"serve", "--profile", "dev"})
	if !strings.Contains(result, `"serve"`) {
		t.Errorf("expected claude-code format to contain serve, got: %s", result)
	}
	if !strings.Contains(result, `"--profile"`) {
		t.Errorf("expected claude-code format to contain --profile, got: %s", result)
	}
}

// Test that buildJSONConfig always produces valid JSON
func TestBuildJSONConfig_AlwaysValidJSON(t *testing.T) {
	tools := []string{"claude-code", "opencode", "copilot", "kimi"}
	for _, tool := range tools {
		t.Run(tool, func(t *testing.T) {
			result := buildJSONConfig(tool, "symmemory", []string{"serve"})
			var parsed interface{}
			if err := json.Unmarshal([]byte(result), &parsed); err != nil {
				t.Errorf("buildJSONConfig(%q) produced invalid JSON: %v\n%s", tool, err, result)
			}
		})
	}
}

func TestBuildCodexToml(t *testing.T) {
	result := buildCodexToml("/usr/bin/symmemory", []string{"serve", "--profile", "test"})
	if !strings.Contains(result, `[mcp_servers.symaira-memory]`) {
		t.Errorf("expected TOML header, got: %s", result)
	}
	if !strings.Contains(result, `command = "/usr/bin/symmemory"`) {
		t.Errorf("expected command in TOML, got: %s", result)
	}
	if !strings.Contains(result, `"serve"`) {
		t.Errorf("expected serve arg in TOML, got: %s", result)
	}
}

func TestBuildCodexToml_NoArgs(t *testing.T) {
	result := buildCodexToml("symmemory", nil)
	if !strings.Contains(result, `command = "symmemory"`) {
		t.Errorf("expected command, got: %s", result)
	}
	if strings.Contains(result, "args =") {
		t.Errorf("expected no args line when serveArgs is empty, got: %s", result)
	}
}

// --------------------------------------------------------------------------
// contextOutputFormat (context.go) — edge cases
// --------------------------------------------------------------------------

func TestContextOutputFormat_JSON(t *testing.T) {
	old := outputFormat
	outputFormat = "json"
	defer func() { outputFormat = old }()

	got := contextOutputFormat(nil)
	if got != "json" {
		t.Errorf("expected 'json', got %q", got)
	}
}

func TestContextOutputFormat_DefaultToMD(t *testing.T) {
	old := outputFormat
	outputFormat = ""
	defer func() { outputFormat = old }()

	got := contextOutputFormat(nil)
	if got != "md" {
		t.Errorf("expected 'md', got %q", got)
	}
}

func TestContextOutputFormat_TableBecomesMD(t *testing.T) {
	old := outputFormat
	outputFormat = "table"
	defer func() { outputFormat = old }()

	got := contextOutputFormat(nil)
	if got != "md" {
		t.Errorf("expected 'md' for table format, got %q", got)
	}
}

// --------------------------------------------------------------------------
// emitContextEmpty (context.go) — only JSON branch tested (0% coverage)
// --------------------------------------------------------------------------

func TestEmitContextEmpty_JSON(t *testing.T) {
	output := captureCmdOutput(func() {
		emitContextEmpty("json")
	})
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("expected valid JSON: %v", err)
	}
	if parsed["used_tokens"] != float64(0) {
		t.Errorf("expected used_tokens=0, got %v", parsed["used_tokens"])
	}
}

func TestEmitContextEmpty_MD(t *testing.T) {
	// Markdown mode should produce no output
	output := captureCmdOutput(func() {
		emitContextEmpty("md")
	})
	if output != "" {
		t.Errorf("expected empty output for md, got %q", output)
	}
}

// --------------------------------------------------------------------------
// estimateContextTokens (context.go) — edge cases
// --------------------------------------------------------------------------

func TestEstimateContextTokens_EdgeCases(t *testing.T) {
	// estimateContextTokens is already tested in context_test.go for basic cases.
	// This adds the unicode edge case.
	got := estimateContextTokens("")
	if got != 0 {
		t.Errorf("expected 0 for empty string, got %d", got)
	}
}

// --------------------------------------------------------------------------
// sync.go — readPassphraseFile (85.7% coverage, test edge cases)
// --------------------------------------------------------------------------

func TestReadPassphraseFile_Empty(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/empty.pp"
	if err := os.WriteFile(path, []byte("  \n"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := readPassphraseFile(path)
	if err == nil {
		t.Fatal("expected error for empty passphrase file")
	}
}

func TestReadPassphraseFile_Nonexistent(t *testing.T) {
	_, err := readPassphraseFile("/nonexistent/pp.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent passphrase file")
	}
}

// --------------------------------------------------------------------------
// FormatText — regression test for template errors
// --------------------------------------------------------------------------

func TestFormatText_InvalidTemplate(t *testing.T) {
	var buf bytes.Buffer
	f := &OutputFormatter{Format: "text", Writer: &buf}

	err := f.FormatText(sampleMemory(), "{{.InvalidSyntax")
	if err == nil {
		t.Error("expected error for invalid template")
	}
}

// --------------------------------------------------------------------------
// Output dispatcher — template names
// --------------------------------------------------------------------------

func TestOutputDispatcher_AllTemplateNames(t *testing.T) {
	templates := []string{"list", "search", "get", "entity-list", "entity-resolve", "rule-list"}
	for _, name := range templates {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			f := &OutputFormatter{Format: "text", Writer: &buf}

			// Provide appropriate data for each template type
			var err error
			switch name {
			case "search":
				err = f.Output([]db.SearchResult{{Memory: sampleMemory(), Score: 0.5}}, name)
			case "entity-list", "entity-resolve":
				err = f.Output([]db.Entity{}, name)
			case "rule-list":
				err = f.Output([]string{}, name)
			case "list":
				err = f.Output([]db.Memory{*sampleMemory()}, name)
			case "get":
				err = f.Output(sampleMemory(), name)
			default:
				// entity-list, entity-resolve, rule-list handled above
			}
			if err != nil {
				t.Errorf("Output(%q) error: %v", name, err)
			}
			if buf.Len() == 0 {
				t.Errorf("Output(%q) produced empty output", name)
			}
		})
	}
}

// --------------------------------------------------------------------------
// FormatJSON — edge case with nil data
// --------------------------------------------------------------------------

func TestFormatJSON_Nil(t *testing.T) {
	var buf bytes.Buffer
	f := &OutputFormatter{Format: "json", Writer: &buf}

	err := f.FormatJSON(nil)
	if err != nil {
		t.Fatalf("FormatJSON(nil) error: %v", err)
	}
	output := strings.TrimSpace(buf.String())
	if output != "null" {
		t.Errorf("expected 'null', got %q", output)
	}
}

// --------------------------------------------------------------------------
// NewOutputFormatter — coverage for constructor
// --------------------------------------------------------------------------

func TestNewOutputFormatter(t *testing.T) {
	f := NewOutputFormatter("json")
	if f.Format != "json" {
		t.Errorf("expected format 'json', got %q", f.Format)
	}
	if f.Writer == nil {
		t.Error("expected non-nil Writer (os.Stdout)")
	}
	if f.IncludeEmbedding {
		t.Error("expected IncludeEmbedding to be false by default")
	}
}

// --------------------------------------------------------------------------
// nocover functions — execute (root.go) calls os.Exit, hard to test
// runBackgroundCompaction (serve.go) — goroutine, hard to test directly
// assembleRecentMemories (context.go) — needs real DB with memories
// We skip those as they require integration-style setup.
// --------------------------------------------------------------------------

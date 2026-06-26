package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "init" {
			found = true
			break
		}
	}
	if !found {
		t.Error("init command not registered")
	}
}

func TestInitCommandFlags(t *testing.T) {
	initCmd := findSubcommand(rootCmd, "init")
	if initCmd == nil {
		t.Fatal("init command not found")
	}

	fileFlag := initCmd.Flags().Lookup("file")
	if fileFlag == nil {
		t.Fatal("expected 'file' flag on init command")
	}
	if fileFlag.DefValue != "AGENTS.md" {
		t.Errorf("expected default 'AGENTS.md', got %q", fileFlag.DefValue)
	}

	dryRunFlag := initCmd.Flags().Lookup("dry-run")
	if dryRunFlag == nil {
		t.Fatal("expected 'dry-run' flag on init command")
	}
	if dryRunFlag.DefValue != "false" {
		t.Errorf("expected default false, got %q", dryRunFlag.DefValue)
	}
}

func TestInitPersistentPreRunBypassesDB(t *testing.T) {
	initCmd := findSubcommand(rootCmd, "init")
	if initCmd == nil {
		t.Fatal("init command not found")
	}
	err := rootCmd.PersistentPreRunE(initCmd, nil)
	if err != nil {
		t.Errorf("init should bypass DB in PersistentPreRunE: %v", err)
	}
}

func TestUpdateAGENTSContent_InsertIntoEmpty(t *testing.T) {
	block := managedBlock("test content")
	result := updateAGENTSContent("", block)

	if !strings.Contains(result, markerStart) {
		t.Error("expected start marker in result")
	}
	if !strings.Contains(result, markerEnd) {
		t.Error("expected end marker in result")
	}
	if !strings.Contains(result, "test content") {
		t.Error("expected content in result")
	}
}

func TestUpdateAGENTSContent_AppendToExisting(t *testing.T) {
	existing := "# My Project\n\nSome notes here.\n"
	block := managedBlock("new content")
	result := updateAGENTSContent(existing, block)

	if !strings.HasPrefix(result, "# My Project") {
		t.Error("expected existing content preserved at start")
	}
	if !strings.Contains(result, "Some notes here.") {
		t.Error("expected existing notes preserved")
	}
	if !strings.Contains(result, markerStart) {
		t.Error("expected start marker in result")
	}
	if !strings.Contains(result, "new content") {
		t.Error("expected new content in result")
	}
	// Block should come after existing content
	startIdx := strings.Index(result, markerStart)
	existingIdx := strings.Index(result, "Some notes here.")
	if startIdx < existingIdx {
		t.Error("expected managed block after existing content")
	}
}

func TestUpdateAGENTSContent_Idempotent(t *testing.T) {
	block := managedBlock("content v1")
	afterFirst := updateAGENTSContent("", block)

	// Re-insert with different content — should replace, not duplicate
	block2 := managedBlock("content v2")
	afterSecond := updateAGENTSContent(afterFirst, block2)

	// Should contain v2 but not v1
	if !strings.Contains(afterSecond, "content v2") {
		t.Error("expected updated content v2")
	}
	if strings.Contains(afterSecond, "content v1") {
		t.Error("expected old content v1 to be replaced")
	}

	// Should have exactly one start marker
	if strings.Count(afterSecond, markerStart) != 1 {
		t.Errorf("expected exactly 1 start marker, got %d", strings.Count(afterSecond, markerStart))
	}
	if strings.Count(afterSecond, markerEnd) != 1 {
		t.Errorf("expected exactly 1 end marker, got %d", strings.Count(afterSecond, markerEnd))
	}
}

func TestUpdateAGENTSContent_PreservesSurroundingContent(t *testing.T) {
	existing := "# Header\n\nExisting paragraph.\n\n## Section\n\nSome text.\n"
	block := managedBlock("managed content")
	result := updateAGENTSContent(existing, block)

	// All original content should be present
	for _, part := range []string{"# Header", "Existing paragraph.", "## Section", "Some text."} {
		if !strings.Contains(result, part) {
			t.Errorf("expected %q to be preserved", part)
		}
	}

	// Managed block should be present
	if !strings.Contains(result, "managed content") {
		t.Error("expected managed content in result")
	}
}

func TestUpdateAGENTSContent_ReplaceExistingBlock(t *testing.T) {
	existing := "# Header\n\n" + markerStart + "\nold content\n" + markerEnd + "\n\nFooter.\n"
	block := managedBlock("new content")
	result := updateAGENTSContent(existing, block)

	if !strings.Contains(result, "new content") {
		t.Error("expected new content in result")
	}
	if strings.Contains(result, "old content") {
		t.Error("expected old content to be replaced")
	}
	if !strings.Contains(result, "# Header") {
		t.Error("expected header preserved")
	}
	if !strings.Contains(result, "Footer.") {
		t.Error("expected footer preserved")
	}
	if strings.Count(result, markerStart) != 1 {
		t.Errorf("expected exactly 1 start marker, got %d", strings.Count(result, markerStart))
	}
}

func TestManagedBlock_WrapsCorrectly(t *testing.T) {
	block := managedBlock("hello")
	if !strings.HasPrefix(block, markerStart+"\n") {
		t.Error("expected block to start with start marker + newline")
	}
	if !strings.HasSuffix(block, markerEnd+"\n") {
		t.Error("expected block to end with end marker + newline")
	}
	if !strings.Contains(block, "hello") {
		t.Error("expected content inside markers")
	}
}

func TestInitCreatesFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-init-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	targetFile := filepath.Join(tempDir, "AGENTS.md")

	// Save and restore flags
	origFile := initFile
	origDryRun := initDryRun
	defer func() {
		initFile = origFile
		initDryRun = origDryRun
	}()

	initFile = targetFile
	initDryRun = false

	err = initCmd.RunE(initCmd, nil)
	if err != nil {
		t.Fatalf("init command failed: %v", err)
	}

	data, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("failed to read created file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, markerStart) {
		t.Error("expected start marker in created file")
	}
	if !strings.Contains(content, markerEnd) {
		t.Error("expected end marker in created file")
	}
	if !strings.Contains(content, "Symaira Memory") {
		t.Error("expected Symaira Memory content in file")
	}
}

func TestInitDryRunDoesNotWrite(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-init-dryrun-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	targetFile := filepath.Join(tempDir, "AGENTS.md")

	origFile := initFile
	origDryRun := initDryRun
	defer func() {
		initFile = origFile
		initDryRun = origDryRun
	}()

	initFile = targetFile
	initDryRun = true

	stdout := captureCmdOutput(func() {
		err := initCmd.RunE(initCmd, nil)
		if err != nil {
			t.Errorf("dry-run failed: %v", err)
		}
	})

	// File should NOT exist
	if _, err := os.Stat(targetFile); !os.IsNotExist(err) {
		t.Error("dry-run should not create file on disk")
	}

	// stdout should contain the managed block
	if !strings.Contains(stdout, markerStart) {
		t.Error("dry-run should print content to stdout")
	}
}

func TestInitIdempotentRun(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-init-idempotent-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	targetFile := filepath.Join(tempDir, "AGENTS.md")

	origFile := initFile
	origDryRun := initDryRun
	defer func() {
		initFile = origFile
		initDryRun = origDryRun
	}()

	initFile = targetFile
	initDryRun = false

	// First run: create
	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("first run failed: %v", err)
	}

	data1, _ := os.ReadFile(targetFile)

	// Second run: update (idempotent)
	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("second run failed: %v", err)
	}

	data2, _ := os.ReadFile(targetFile)

	if string(data1) != string(data2) {
		t.Error("expected identical output from idempotent run")
	}

	// Verify exactly one block
	if strings.Count(string(data2), markerStart) != 1 {
		t.Errorf("expected exactly 1 start marker after idempotent run, got %d",
			strings.Count(string(data2), markerStart))
	}
}

func TestInitPreservesExistingContent(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "symmemory-init-preserve-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	targetFile := filepath.Join(tempDir, "AGENTS.md")
	existingContent := "# My Project\n\nThis is my project's AGENTS.md.\n\n## Guidelines\n\nBe nice.\n"
	if err := os.WriteFile(targetFile, []byte(existingContent), 0644); err != nil {
		t.Fatalf("failed to write existing file: %v", err)
	}

	origFile := initFile
	origDryRun := initDryRun
	defer func() {
		initFile = origFile
		initDryRun = origDryRun
	}()

	initFile = targetFile
	initDryRun = false

	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	data, _ := os.ReadFile(targetFile)
	content := string(data)

	// Original content preserved
	if !strings.Contains(content, "# My Project") {
		t.Error("expected original header preserved")
	}
	if !strings.Contains(content, "Be nice.") {
		t.Error("expected original guideline preserved")
	}

	// Managed block appended
	if !strings.Contains(content, markerStart) {
		t.Error("expected managed block appended")
	}
}

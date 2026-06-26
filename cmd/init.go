package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/danieljustus/symaira-memory/internal/instructions"
	"github.com/spf13/cobra"
)

const (
	markerStart = "<!-- symmemory:start -->"
	markerEnd   = "<!-- symmemory:end -->"
)

var initFile string
var initDryRun bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Insert Symaira Memory agent instructions into AGENTS.md",
	Long: `Insert a managed block of agent integration instructions into AGENTS.md
(or the file specified by --file). The block is idempotent: re-running replaces
the existing managed block, leaving surrounding content untouched.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		content := instructions.Text(Version)
		newBlock := managedBlock(content)

		existing := ""
		data, err := os.ReadFile(initFile)
		if err == nil {
			existing = string(data)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to read %s: %w", initFile, err)
		}

		result := updateAGENTSContent(existing, newBlock)

		if initDryRun {
			fmt.Fprint(os.Stdout, result)
			return nil
		}

		if err := os.WriteFile(initFile, []byte(result), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", initFile, err)
		}

		if existing == "" {
			fmt.Fprintf(os.Stderr, "Created %s with Symaira Memory agent instructions.\n", initFile)
		} else if strings.Contains(existing, markerStart) {
			fmt.Fprintf(os.Stderr, "Updated Symaira Memory block in %s.\n", initFile)
		} else {
			fmt.Fprintf(os.Stderr, "Appended Symaira Memory block to %s.\n", initFile)
		}

		return nil
	},
}

// managedBlock wraps content between the start and end markers.
func managedBlock(content string) string {
	return markerStart + "\n" + content + "\n" + markerEnd + "\n"
}

// updateAGENTSContent replaces or appends the managed block in existing content.
func updateAGENTSContent(existing, newBlock string) string {
	startIdx := strings.Index(existing, markerStart)
	endIdx := strings.Index(existing, markerEnd)

	if startIdx >= 0 && endIdx > startIdx {
		before := existing[:startIdx]
		after := strings.TrimPrefix(existing[endIdx+len(markerEnd):], "\n")
		return before + newBlock + after
	}

	// Append new block
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	return existing + "\n" + newBlock
}

func init() {
	initCmd.Flags().StringVar(&initFile, "file", "AGENTS.md", "Target file path")
	initCmd.Flags().BoolVar(&initDryRun, "dry-run", false, "Print what would be written without modifying files")
	rootCmd.AddCommand(initCmd)
}

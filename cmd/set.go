package cmd

import (
	"fmt"
	"os"
	"os/user"

	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/danieljustus/symaira-memory/internal/memory"
	"github.com/spf13/cobra"
)

var (
	setValue  string
	setScope  string
	setAuthor string
	setSession string
)

func init() {
	setCmd.Flags().StringVarP(&setValue, "value", "v", "", "Content/fact text of the memory to save")
	setCmd.Flags().StringVarP(&setScope, "scope", "s", "global", "Scope level: global, project, agent, user, session")
	setCmd.Flags().StringVar(&setAuthor, "author", "", "Author attribution (default: cli:$USER)")
	setCmd.Flags().StringVar(&setSession, "session", "", "Session ID attribution")
	_ = setCmd.MarkFlagRequired("value")
	rootCmd.AddCommand(setCmd)
}

var setCmd = &cobra.Command{
	Use:   "set",
	Short: "Save a new fact or context snippet into persistent memory",
	Long: `Save a new fact or context snippet to local SQLite storage. 
Automatically triggers embedding generation, PII redaction, and project scope detection.`,
	Run: func(cmd *cobra.Command, args []string) {
		author := setAuthor
		if author == "" {
			if u, err := user.Current(); err == nil && u.Username != "" {
				author = "cli:" + u.Username
			} else {
				author = "cli:unknown"
			}
		}
		attr := memory.Attribution{
			Author:    author,
			SessionID: setSession,
		}

		meta := map[string]string{"source": "cli_set"}
		embeddings := extractor.NewEmbeddingsGenerator(GetConfig())
		patternExtractor := extractor.NewPatternExtractor()

		m, _, err := memory.Store(GetDB(), embeddings, patternExtractor, setValue, setScope, meta, true, attr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("⚡ Memory saved successfully!\n")
		fmt.Printf("  ID:      %s\n", m.ID)
		fmt.Printf("  Content: %s\n", m.Content)
		fmt.Printf("  Scope:   %s\n", m.Scope)
		if m.Scope == "project" {
			fmt.Printf("  Project: %s\n", m.Metadata["project_name"])
		}
		if m.CreatedBy != "" {
			fmt.Printf("  Author:  %s\n", m.CreatedBy)
		}
		if m.CreatedSession != "" {
			fmt.Printf("  Session: %s\n", m.CreatedSession)
		}
	},
}

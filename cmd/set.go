package cmd

import (
	"fmt"
	"os"

	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/danieljustus/symaira-memory/internal/memory"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var (
	setValue string
	setScope string
)

func init() {
	setCmd.Flags().StringVarP(&setValue, "value", "v", "", "Content/fact text of the memory to save")
	setCmd.Flags().StringVarP(&setScope, "scope", "s", "global", "Scope level: global, project, agent, user, session")
	_ = setCmd.MarkFlagRequired("value")
	rootCmd.AddCommand(setCmd)
}

var setCmd = &cobra.Command{
	Use:   "set",
	Short: "Save a new fact or context snippet into persistent memory",
	Long: `Save a new fact or context snippet to local SQLite storage. 
Automatically triggers embedding generation, PII redaction, and project scope detection.`,
	Run: func(cmd *cobra.Command, args []string) {
		meta := map[string]string{"source": "cli_set"}
		m, err := memory.Prepare(setValue, setScope, meta, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		m.ID = uuid.New().String()

		embeddings := extractor.NewEmbeddingsGenerator(GetConfig())
		m.Embedding = embeddings.GenerateVector(m.Content)

		if err := GetDB().SaveMemory(m); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing memory to SQLite: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("⚡ Memory saved successfully!\n")
		fmt.Printf("  ID:      %s\n", m.ID)
		fmt.Printf("  Content: %s\n", m.Content)
		fmt.Printf("  Scope:   %s\n", m.Scope)
		if m.Scope == "project" {
			fmt.Printf("  Project: %s\n", m.Metadata["project_name"])
		}
	},
}

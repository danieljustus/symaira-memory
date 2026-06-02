package cmd

import (
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
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
Automatically triggers embedding generation and offline pattern-matching fact extraction.`,
	Run: func(cmd *cobra.Command, args []string) {
		embeddings := extractor.NewEmbeddingsGenerator()
		vector := embeddings.GenerateVector(setValue)

		id := uuid.New().String()
		m := &db.Memory{
			ID:        id,
			Content:   setValue,
			Scope:     setScope,
			Metadata:  map[string]string{"source": "cli_set"},
			Embedding: vector,
		}

		if err := RootDB.SaveMemory(m); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing memory to SQLite: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("⚡ Memory saved successfully!\n")
		fmt.Printf("  ID:      %s\n", id)
		fmt.Printf("  Content: %s\n", setValue)
		fmt.Printf("  Scope:   %s\n", setScope)
	},
}

package cmd

import (
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/danieljustus/symaira-memory/internal/security"
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
		if err := security.ValidateScope(setScope); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// Security Integration: PII Guard Redaction
		piiGuard := security.NewPIIGuard()
		cleanValue := piiGuard.Redact(setValue)

		meta := map[string]string{"source": "cli_set"}

		// Security Integration: Active Project Scope detection
		if setScope == "project" {
			detector := security.NewProjectScopeDetector()
			projName := detector.DetectActiveProject()
			meta["project_name"] = projName
		}

		embeddings := extractor.NewEmbeddingsGenerator()
		vector := embeddings.GenerateVector(cleanValue)

		id := uuid.New().String()
		m := &db.Memory{
			ID:        id,
			Content:   cleanValue,
			Scope:     setScope,
			Metadata:  meta,
			Embedding: vector,
		}

		if err := GetDB().SaveMemory(m); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing memory to SQLite: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("⚡ Memory saved successfully!\n")
		fmt.Printf("  ID:      %s\n", id)
		fmt.Printf("  Content: %s\n", cleanValue)
		fmt.Printf("  Scope:   %s\n", setScope)
		if setScope == "project" {
			fmt.Printf("  Project: %s\n", meta["project_name"])
		}
	},
}

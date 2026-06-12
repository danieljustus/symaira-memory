package cmd

import (
	"fmt"
	"os"
	"os/user"
	"strings"

	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/danieljustus/symaira-memory/internal/memory"
	"github.com/spf13/cobra"
)

var (
	setValue    string
	setScope    string
	setAuthor   string
	setSession  string
	setEntities string
)

func init() {
	setCmd.Flags().StringVarP(&setValue, "value", "v", "", "Content/fact text of the memory to save")
	setCmd.Flags().StringVarP(&setScope, "scope", "s", "global", "Scope level: global, project, agent, user, session")
	setCmd.Flags().StringVar(&setAuthor, "author", "", "Author attribution (default: cli:$USER)")
	setCmd.Flags().StringVar(&setSession, "session", "", "Session ID attribution")
	setCmd.Flags().StringVar(&setEntities, "entities", "", "Comma-separated entity names to link (e.g. \"Irene,Premium BnB\")")
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

		var entities []string
		if setEntities != "" {
			for _, e := range strings.Split(setEntities, ",") {
				e = strings.TrimSpace(e)
				if e != "" {
					entities = append(entities, e)
				}
			}
		}

		m, _, err := memory.Store(GetDB(), embeddings, patternExtractor, setValue, setScope, meta, true, attr, entities)
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
		if len(m.Entities) > 0 {
			fmt.Printf("  Entities: %s\n", strings.Join(m.Entities, ", "))
		}
	},
}

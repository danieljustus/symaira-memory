package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"strings"

	"github.com/danieljustus/symaira-corekit/exitcodes"
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
	rootCmd.AddCommand(setCmd)
}

var setCmd = &cobra.Command{
	Use:   "set [content]",
	Short: "Save a new fact or context snippet into persistent memory",
	Long: `Save a new fact or context snippet to local SQLite storage. 
Automatically triggers embedding generation, PII redaction, and project scope detection.`,
	Example: `  # Save a global memory
  symmemory set "Alice prefers dark mode in all applications."

  # Save a project-scoped memory linked to entities
  symmemory set "The API uses JWT auth with 15-minute expiry" -s project --entities "BackendAPI,AuthModule"

  # Save a user-scoped memory with custom author
  symmemory set "Prefers concise commit messages" -s user --author "team-lead"

  # The legacy --value flag is still supported
  symmemory set --value "Alice prefers dark mode in all applications."`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		content := setValue
		if len(args) > 0 {
			if setValue != "" {
				return exitcodes.Wrapf(nil, exitcodes.ExitData, exitcodes.KindValidation, "provide memory content either as a positional argument or with --value, not both")
			}
			content = args[0]
		}
		if content == "" {
			return exitcodes.Wrapf(nil, exitcodes.ExitData, exitcodes.KindValidation, "memory content is required: pass it as a positional argument or use --value")
		}

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

		m, secondaryIDs, err := memory.Store(GetDB(), embeddings, patternExtractor, content, setScope, meta, true, attr, entities, "cli")
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to store memory")
		}

		if GetOutputFormat(cmd) == "json" {
			out := struct {
				ID             string            `json:"id"`
				Content        string            `json:"content"`
				Scope          string            `json:"scope"`
				Metadata       map[string]string `json:"metadata"`
				Author         string            `json:"author,omitempty"`
				Session        string            `json:"session,omitempty"`
				Entities       []string          `json:"entities,omitempty"`
				SecondaryFacts []string          `json:"secondary_facts,omitempty"`
			}{
				ID:             m.ID,
				Content:        m.Content,
				Scope:          m.Scope,
				Metadata:       m.Metadata,
				Author:         m.CreatedBy,
				Session:        m.CreatedSession,
				Entities:       m.Entities,
				SecondaryFacts: secondaryIDs,
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(out); err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to encode JSON output")
			}
			return nil
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
		return nil
	},
}

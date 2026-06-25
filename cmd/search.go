package cmd

import (
	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/spf13/cobra"
)

var (
	searchScope  string
	searchLimit  int
	searchEntity string
	searchFormat string
)

func init() {
	searchCmd.Flags().StringVarP(&searchScope, "scope", "s", "", "Filter search by scope level")
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "l", 5, "Maximum number of search results to return")
	searchCmd.Flags().StringVar(&searchEntity, "entity", "", "Filter search by entity name")
	searchCmd.Flags().StringVar(&searchFormat, "format", "text", "Output format: json or text")
	rootCmd.AddCommand(searchCmd)
}

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Perform semantic query comparison over stored memories offline",
	Example: `  # Search with default limit (5 results)
  symmemory search "preferred theme settings"

  # Return more results with --limit
  symmemory search "authentication flow" --limit 10

  # Filter by scope and entity
  symmemory search "API design decisions" -s project --entity "BackendAPI"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := args[0]
		embeddings := extractor.NewEmbeddingsGenerator(GetConfig())
		emb := embeddings.GenerateVector(query)

		var entityID string
		if searchEntity != "" {
			entity, err := GetDB().ResolveEntity(searchEntity)
			if err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "entity lookup error")
			}
			if entity == nil {
				return exitcodes.Wrapf(nil, exitcodes.ExitNotFound, exitcodes.KindNotFound, "entity not found: %s", searchEntity)
			}
			entityID = entity.ID
		}

		var results []db.SearchResult
		var err error

		if emb.Source == "hash-fallback" {
			// No Ollama available: use BM25/FTS5 keyword search instead of broken vector search
			results, err = GetDB().SearchMemoriesBM25(query, searchScope, searchLimit)
		} else {
			results, err = GetDB().SearchMemoriesFiltered(emb.Vector, emb.Source, searchScope, searchLimit, entityID)
		}
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "semantic search failure")
		}

		formatter := NewOutputFormatter(GetOutputFormat(cmd))
		if err := formatter.Output(results, "search"); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "output error")
		}
		return nil
	},
}

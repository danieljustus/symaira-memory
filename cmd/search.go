package cmd

import (
	"fmt"
	"os"

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
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		query := args[0]
		embeddings := extractor.NewEmbeddingsGenerator(GetConfig())
		queryVector := embeddings.GenerateVector(query)

		var entityID string
		if searchEntity != "" {
			entity, err := GetDB().ResolveEntity(searchEntity)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Entity lookup error: %v\n", err)
				os.Exit(1)
			}
			if entity == nil {
				fmt.Fprintf(os.Stderr, "Entity not found: %s\n", searchEntity)
				os.Exit(1)
			}
			entityID = entity.ID
		}

		results, err := GetDB().SearchMemoriesFiltered(queryVector, searchScope, searchLimit, entityID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Semantic search failure: %v\n", err)
			os.Exit(1)
		}

		formatter := NewOutputFormatter(searchFormat)
		if err := formatter.Output(results, "search"); err != nil {
			fmt.Fprintf(os.Stderr, "Output error: %v\n", err)
			os.Exit(1)
		}
	},
}

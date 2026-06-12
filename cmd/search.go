package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/spf13/cobra"
)

var (
	searchScope  string
	searchLimit  int
	searchEntity string
)

func init() {
	searchCmd.Flags().StringVarP(&searchScope, "scope", "s", "", "Filter search by scope level")
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "l", 5, "Maximum number of search results to return")
	searchCmd.Flags().StringVar(&searchEntity, "entity", "", "Filter search by entity name")
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

		if len(results) == 0 {
			fmt.Println("No relevant memories found.")
			return
		}

		bytes, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to encode results: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(bytes))
	},
}

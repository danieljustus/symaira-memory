package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	listScope  string
	listEntity string
	listFormat string
)

func init() {
	listCmd.Flags().StringVarP(&listScope, "scope", "s", "", "Filter list by scopes level")
	listCmd.Flags().StringVar(&listEntity, "entity", "", "Filter list by entity name")
	listCmd.Flags().StringVar(&listFormat, "format", "text", "Output format: json or text")
	rootCmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all stored memory entries in the database",
	Example: `  # List all memories
  symmemory list

  # Filter by scope
  symmemory list --scope project

  # Filter by entity and output as JSON
  symmemory list --entity "BackendAPI" --format json`,
	Run: func(cmd *cobra.Command, args []string) {
		var entityID string
		if listEntity != "" {
			entity, err := GetDB().ResolveEntity(listEntity)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Entity lookup error: %v\n", err)
				os.Exit(1)
			}
			if entity == nil {
				fmt.Fprintf(os.Stderr, "Entity not found: %s\n", listEntity)
				os.Exit(1)
			}
			entityID = entity.ID
		}

		mems, err := GetDB().ListMemoriesFiltered(listScope, entityID, 0, 1000)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Database read failure: %v\n", err)
			os.Exit(1)
		}

		formatter := NewOutputFormatter(listFormat)
		if err := formatter.Output(mems, "list"); err != nil {
			fmt.Fprintf(os.Stderr, "Output error: %v\n", err)
			os.Exit(1)
		}
	},
}

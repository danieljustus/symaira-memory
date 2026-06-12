package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	listScope  string
	listEntity string
)

func init() {
	listCmd.Flags().StringVarP(&listScope, "scope", "s", "", "Filter list by scope level")
	listCmd.Flags().StringVar(&listEntity, "entity", "", "Filter list by entity name")
	rootCmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all stored memory entries in the database",
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

		if len(mems) == 0 {
			fmt.Println("[]")
			return
		}

		bytes, err := json.MarshalIndent(mems, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to encode memories: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(bytes))
	},
}

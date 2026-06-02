package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(getCmd)
}

var getCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Retrieve detailed JSON representation of a stored memory by ID",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		m, err := RootDB.GetMemory(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Database read error: %v\n", err)
			os.Exit(1)
		}

		if m == nil {
			fmt.Fprintf(os.Stderr, "Memory not found with ID: %s\n", id)
			os.Exit(1)
		}

		bytes, _ := json.MarshalIndent(m, "", "  ")
		fmt.Println(string(bytes))
	},
}

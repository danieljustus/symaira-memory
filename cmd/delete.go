package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(deleteCmd)
}

var deleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Permanently remove a stored memory by ID",
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

		if err := RootDB.DeleteMemory(id); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to delete memory: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Memory %s permanently deleted.\n", id)
	},
}

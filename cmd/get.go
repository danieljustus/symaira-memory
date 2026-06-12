package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var getFormat string

func init() {
	getCmd.Flags().StringVar(&getFormat, "format", "text", "Output format: json or text")
	rootCmd.AddCommand(getCmd)
}

var getCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Retrieve detailed representation of a stored memory by ID",
	Example: `  # Retrieve a memory by ID
  symmemory get mem_abc123def456

  # Output as JSON for scripting
  symmemory get mem_abc123def456 --format json`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		m, err := GetDB().GetMemory(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Database read error: %v\n", err)
			os.Exit(1)
		}

		if m == nil {
			fmt.Fprintf(os.Stderr, "Memory not found with ID: %s\n", id)
			os.Exit(1)
		}

		formatter := NewOutputFormatter(getFormat)
		if err := formatter.Output(m, "get"); err != nil {
			fmt.Fprintf(os.Stderr, "Output error: %v\n", err)
			os.Exit(1)
		}
	},
}

package cmd

import (
	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/spf13/cobra"
)

var getFormat string
var getIncludeEmbedding bool

func init() {
	getCmd.Flags().StringVar(&getFormat, "format", "text", "Output format: json or text")
	getCmd.Flags().BoolVar(&getIncludeEmbedding, "include-embedding", false, "Include raw embedding vectors in JSON output (omitted by default)")
	rootCmd.AddCommand(getCmd)
}

var getCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Retrieve detailed representation of a stored memory by ID",
	Example: `  # Retrieve a memory by ID
  symmemory get mem_abc123def456

  # Output as JSON for scripting
  symmemory get mem_abc123def456 --format json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		m, err := GetDB().GetMemory(id)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "database read error")
		}

		if m == nil {
			return exitcodes.Wrapf(nil, exitcodes.ExitNotFound, exitcodes.KindNotFound, "memory not found with ID: %s", id)
		}

		formatter := NewOutputFormatter(GetOutputFormat(cmd))
		formatter.IncludeEmbedding = getIncludeEmbedding
		if err := formatter.Output(m, "get"); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "output error")
		}
		return nil
	},
}

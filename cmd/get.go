package cmd

import (
	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/spf13/cobra"
)

var getIncludeEmbedding bool
var getWithEvidence bool

func init() {
	getCmd.Flags().BoolVar(&getIncludeEmbedding, "include-embedding", false, "Include raw embedding vectors in JSON output (omitted by default)")
	getCmd.Flags().BoolVar(&getWithEvidence, "with-evidence", false, "Include grounded evidence spans backing this memory, if any (omitted by default)")
	rootCmd.AddCommand(getCmd)
}

var getCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Retrieve detailed representation of a stored memory by ID",
	Example: `  # Retrieve a memory by ID
  symmemory get mem_abc123def456

  # Output as JSON for scripting
  symmemory get mem_abc123def456 --output json

  # Include grounded evidence spans
  symmemory get mem_abc123def456 --with-evidence --output json`,
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

		if getWithEvidence {
			evidence, err := GetDB().GetMemoryEvidence(id)
			if err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "evidence read error")
			}
			m.Evidence = evidence
		}

		formatter := NewOutputFormatter(GetOutputFormat(cmd))
		formatter.IncludeEmbedding = getIncludeEmbedding
		if err := formatter.Output(m, "get"); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "output error")
		}
		return nil
	},
}

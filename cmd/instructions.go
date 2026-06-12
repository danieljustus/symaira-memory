package cmd

import (
	"fmt"

	"github.com/danieljustus/symaira-memory/internal/instructions"
	"github.com/spf13/cobra"
)

var instructionsCmd = &cobra.Command{
	Use:   "instructions",
	Short: "Print agent integration instructions",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(instructions.Text(Version))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(instructionsCmd)
}

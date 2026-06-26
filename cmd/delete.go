package cmd

import (
	"fmt"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(deleteCmd)
}

var deleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Permanently remove a stored memory by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]

		m, err := GetDB().GetMemory(id)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "database read error")
		}
		if m == nil {
			return exitcodes.Wrapf(nil, exitcodes.ExitNotFound, exitcodes.KindNotFound, "memory not found with ID: %s", id)
		}

		if err := GetDB().DeleteMemory(id); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to delete memory")
		}

		fmt.Printf("Memory %s permanently deleted.\n", id)
		return nil
	},
}

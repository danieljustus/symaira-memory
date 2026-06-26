package cmd

import (
	"github.com/danieljustus/symaira-corekit/exitcodes"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		var entityID string
		if listEntity != "" {
			entity, err := GetDB().ResolveEntity(listEntity)
			if err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "entity lookup error")
			}
			if entity == nil {
				return exitcodes.Wrapf(nil, exitcodes.ExitNotFound, exitcodes.KindNotFound, "entity not found: %s", listEntity)
			}
			entityID = entity.ID
		}

		mems, err := GetDB().ListMemoriesFiltered(listScope, entityID, 0, 1000)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "database read failure")
		}

		formatter := NewOutputFormatter(GetOutputFormat(cmd))
		if err := formatter.Output(mems, "list"); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "output error")
		}
		return nil
	},
}

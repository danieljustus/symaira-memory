package cmd

import (
	"fmt"
	"os/user"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/security"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var (
	ruleScope  string
	ruleAuthor string
)

func init() {
	ruleCmd.AddCommand(ruleAddCmd)
	ruleCmd.AddCommand(ruleListCmd)
	ruleCmd.AddCommand(ruleDeleteCmd)

	ruleCmd.PersistentFlags().StringVarP(&ruleScope, "scope", "s", "global", "Scope level: global, project, agent, user")
	ruleAddCmd.Flags().StringVar(&ruleAuthor, "author", "", "Author attribution (default: cli:$USER)")
	rootCmd.AddCommand(ruleCmd)
}

var ruleCmd = &cobra.Command{
	Use:   "rule",
	Short: "Manage procedural memory rules (behavioral instructions for AI agents)",
	Long:  `Manage behavioral guidelines and procedural rules that are automatically injected into AI prompts.`,
}

var ruleAddCmd = &cobra.Command{
	Use:   "add [instruction]",
	Short: "Add a new behavioral rule",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		instruction := args[0]
		id := uuid.New().String()

		author := ruleAuthor
		if author == "" {
			if u, err := user.Current(); err == nil && u.Username != "" {
				author = "cli:" + u.Username
			} else {
				author = "cli:unknown"
			}
		}

		meta := map[string]string{"source": "cli_rule_add"}
		if ruleScope == "project" {
			detector := security.NewProjectScopeDetector()
			meta["project_name"] = detector.DetectActiveProject()
		}

		r := &db.Rule{
			ID:        id,
			Content:   instruction,
			Scope:     ruleScope,
			Metadata:  meta,
			CreatedBy: author,
			UpdatedBy: author,
		}

		if err := GetDB().SaveRule(r); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to save rule")
		}

		fmt.Printf("⚡ Procedural rule saved successfully!\n")
		fmt.Printf("  ID:          %s\n", id)
		fmt.Printf("  Instruction: %s\n", instruction)
		fmt.Printf("  Scope:       %s\n", ruleScope)
		if ruleScope == "project" {
			fmt.Printf("  Project:     %s\n", meta["project_name"])
		}
		return nil
	},
}

var ruleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all behavioral rules",
	RunE: func(cmd *cobra.Command, args []string) error {
		rules, err := GetDB().ListRules(ruleScope)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "database read failure")
		}

		formatter := NewOutputFormatter(GetOutputFormat(cmd))
		if err := formatter.Output(rules, "rule-list"); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "output error")
		}
		return nil
	},
}

var ruleDeleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete a behavioral rule by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		if err := GetDB().DeleteRule(id); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to delete rule")
		}
		fmt.Printf("⚡ Rule successfully deleted: %s\n", id)
		return nil
	},
}

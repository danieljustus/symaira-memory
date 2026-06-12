package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"

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
	Run: func(cmd *cobra.Command, args []string) {
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
			fmt.Fprintf(os.Stderr, "Error saving rule: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("⚡ Procedural rule saved successfully!\n")
		fmt.Printf("  ID:          %s\n", id)
		fmt.Printf("  Instruction: %s\n", instruction)
		fmt.Printf("  Scope:       %s\n", ruleScope)
		if ruleScope == "project" {
			fmt.Printf("  Project:     %s\n", meta["project_name"])
		}
	},
}

var ruleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all behavioral rules",
	Run: func(cmd *cobra.Command, args []string) {
		rules, err := GetDB().ListRules(ruleScope)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Database read failure: %v\n", err)
			os.Exit(1)
		}

		if len(rules) == 0 {
			fmt.Println("[]")
			return
		}

		bytes, err := json.MarshalIndent(rules, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to encode rules: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(bytes))
	},
}

var ruleDeleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete a behavioral rule by ID",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		if err := GetDB().DeleteRule(id); err != nil {
			fmt.Fprintf(os.Stderr, "Delete error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("⚡ Rule successfully deleted: %s\n", id)
	},
}

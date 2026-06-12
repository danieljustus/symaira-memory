package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-corekit/updatecheck"
	"github.com/spf13/cobra"
)

var (
	Version string = "dev"
	Commit  string = "none"
	Date    string = "unknown"
)

var (
	rootDB  *db.DB
	rootCfg *config.Config
)

func GetDB() *db.DB {
	return rootDB
}

func SetDB(database *db.DB) {
	rootDB = database
}

func GetConfig() *config.Config {
	return rootCfg
}

func SetConfig(cfg *config.Config) {
	rootCfg = cfg
}

var rootCmd = &cobra.Command{
	Use:   "symmemory",
	Short: "Symaira Memory (symmemory) — Context layer for the Human-AI Symbiosis Era",
	Long: `Symaira Memory is a next-generation local persistent context and memory system 
built for AI-Agent workflows. It stores facts, summaries, and scopes offline utilizing 
SQLite, and exposes them to agents through the Model Context Protocol (MCP).`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Name() == "version" || cmd.Name() == "mcp-config" || cmd.Name() == "instructions" || cmd.Name() == "init" || (cmd.Parent() != nil && cmd.Parent().Name() == "mcp-config") {
			return nil
		}
		cfg, err := config.Load()
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitConfig, exitcodes.KindConfig, "failed to load configuration")
		}
		SetConfig(cfg)
		database, err := db.Open(cfg)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to open SQLite database")
		}
		SetDB(database)
		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if db := GetDB(); db != nil {
			db.Close()
		}
	},
}

func SetVersionInfo(v, c, d string) {
	Version = v
	Commit = c
	Date = d
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, exitcodes.FormatCLIError(err))
		os.Exit(int(exitcodes.ExitCodeFromError(err)))
	}
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the current CLI version details",
	Run: func(cmd *cobra.Command, args []string) {
		check, _ := cmd.Flags().GetBool("check")
		if check {
			checker := updatecheck.NewChecker("danieljustus", "symaira-memory")
			release, err := checker.Check(context.Background(), Version)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Update check failed: %v\n", err)
				os.Exit(1)
			}
			if release != nil {
				fmt.Printf("Update available: %s → %s\n", Version, release.TagName)
				fmt.Printf("Download: %s\n", release.HTMLURL)
			} else {
				fmt.Printf("symmemory %s is up to date.\n", Version)
			}
			return
		}
		fmt.Printf("symmemory version %s (%s, date: %s)\n", Version, Commit, Date)
	},
}

func init() {
	versionCmd.Flags().Bool("check", false, "Check for updates via GitHub releases")
}

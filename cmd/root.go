package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-corekit/updatecheck"
	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/spf13/cobra"
)

var (
	Version string = "dev"
	Commit  string = "none"
	Date    string = "unknown"
)

var (
	rootDB       *db.DB
	rootCfg      *config.Config
	outputFormat string // global --output flag: "table" or "json"
	noColor      bool   // global --no-color flag
)

// GetNoColor reports whether ANSI color output is disabled.
// Returns true when either the --no-color flag is set or the
// NO_COLOR environment variable is present (per https://no-color.org/).
func GetNoColor() bool {
	return noColor || os.Getenv("NO_COLOR") != ""
}

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
		if cmd.Name() == "version" || cmd.Name() == "mcp-config" || cmd.Name() == "instructions" || cmd.Name() == "init" || cmd.Name() == "doctor" || cmd.Name() == "context" || cmd.Name() == "hook" || (cmd.Parent() != nil && cmd.Parent().Name() == "mcp-config") || (cmd.Parent() != nil && cmd.Parent().Name() == "hook") {
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

// GetOutputFormat returns the resolved output format for the current command.
// It checks the global --output flag first; if not set or "table", it falls
// back to the command-local --format flag when present.
func GetOutputFormat(cmd *cobra.Command) string {
	if outputFormat != "" && outputFormat != "table" {
		return outputFormat
	}
	if f := cmd.Flags().Lookup("format"); f != nil {
		return f.Value.String()
	}
	return outputFormat
}

func init() {
	rootCmd.PersistentFlags().StringVar(&outputFormat, "output", "table", "Output format: table or json")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable ANSI color codes in output (also respects NO_COLOR env var)")
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the current CLI version details",
	RunE: func(cmd *cobra.Command, args []string) error {
		check, _ := cmd.Flags().GetBool("check")
		if check {
			checker := updatecheck.NewChecker("danieljustus", "symaira-memory")
			release, err := checker.Check(context.Background(), Version)
			if err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "update check failed")
			}
			if release != nil {
				fmt.Printf("Update available: %s → %s\n", Version, release.TagName)
				fmt.Printf("Download: %s\n", release.HTMLURL)
			} else {
				fmt.Printf("symmemory %s is up to date.\n", Version)
			}
			return nil
		}
		fmt.Printf("symmemory version %s (%s, date: %s)\n", Version, Commit, Date)
		return nil
	},
}

func init() {
	versionCmd.Flags().Bool("check", false, "Check for updates via GitHub releases")
}

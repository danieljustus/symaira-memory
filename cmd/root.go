package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/danieljustus/symaira-memory/internal/db"
)

var (
	Version string = "dev"
	Commit  string = "none"
	Date    string = "unknown"
)

var (
	rootDB *db.DB
)

// GetDB returns the current database instance. Returns nil if not yet opened.
func GetDB() *db.DB {
	return rootDB
}

// SetDB sets the database instance for use by commands.
func SetDB(database *db.DB) {
	rootDB = database
}

var rootCmd = &cobra.Command{
	Use:   "symmemory",
	Short: "Symaira Memory (symmemory) — Context layer for the Human-AI Symbiosis Era",
	Long: `Symaira Memory is a next-generation local persistent context and memory system 
built for AI-Agent workflows. It stores facts, summaries, and scopes offline utilizing 
SQLite, and exposes them to agents through the Model Context Protocol (MCP).`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Name() == "version" || cmd.Name() == "mcp-config" || (cmd.Parent() != nil && cmd.Parent().Name() == "mcp-config") {
			return nil
		}
		database, err := db.Open()
		if err != nil {
			return fmt.Errorf("failed to open SQLite database: %w", err)
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
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Add subcommands
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the current CLI version details",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("symmemory version %s (%s, date: %s)\n", Version, Commit, Date)
	},
}

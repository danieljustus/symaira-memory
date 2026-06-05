package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/tui"
)

func init() {
	rootCmd.AddCommand(consoleCmd)
}

var consoleCmd = &cobra.Command{
	Use:   "console",
	Short: "Launch the interactive local memory dashboard (TUI)",
	Long: `Starts the high-performance local console UI (built using Bubble Tea and Lip Gloss) 
to curate, browse, filter, search, and delete persistent memory elements in real time.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, _ := config.Load()
		dbPath := ""
		if cfg.Database.Path != "" {
			dbPath = cfg.Database.Path
		} else {
			dbPath = "~/.local/share/symmemory/default.db"
		}
		if err := tui.RunDashboard(GetDB(), dbPath, cfg.Ollama.URL, cfg.Ollama.Model, cfg.Server.HTTPPort); err != nil {
			fmt.Fprintf(os.Stderr, "TUI runtime error: %v\n", err)
			os.Exit(1)
		}
	},
}

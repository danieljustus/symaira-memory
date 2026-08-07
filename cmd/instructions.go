package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/instructions"
	"github.com/spf13/cobra"
)

var (
	instructionsClient string
	instructionsOutput string
	instructionsScope  string
)

var instructionsCmd = &cobra.Command{
	Use:   "instructions",
	Short: "Print agent integration instructions",
	Long: `Print the agent integration guide. Without flags the canonical embedded
document is printed, byte-identical to the tracked skills/symmemory/SKILL.md.

With --client <name> the output is composed from the integration text plus
the stored behavioral rules (symmemory rule add) for the resolved scope, in
the file format that client reads. With --output <path> the rendered
document is written to that path instead of printed. Run 'symmemory
instructions --help' or 'symmemory hook --list' for the client list.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if instructionsClient == "" {
			fmt.Println(instructions.Text(Version))
			return nil
		}

		if !instructions.ValidClient(instructionsClient) {
			return exitcodes.Wrapf(nil, exitcodes.ExitNoInput, exitcodes.KindValidation,
				"unknown client %q; supported clients: %s",
				instructionsClient, strings.Join(instructions.ClientNames(), ", "))
		}

		// The default path is DB-free; open the database lazily for the
		// rules lookup in client mode.
		database := GetDB()
		if database == nil {
			cfg, err := config.Load()
			if err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitConfig, exitcodes.KindConfig, "failed to load configuration")
			}
			database, err = db.Open(cfg)
			if err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to open SQLite database")
			}
			SetDB(database) // PersistentPostRun closes it
		}

		rules, err := database.ListRules(instructionsScope)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to read stored rules")
		}

		doc, err := instructions.Render(Version, instructionsClient, rules)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitNoInput, exitcodes.KindValidation, "render failed")
		}

		if instructionsOutput != "" {
			if err := os.WriteFile(instructionsOutput, []byte(doc), 0644); err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to write output file")
			}
			fmt.Fprintf(os.Stderr, "Instructions written to %s\n", instructionsOutput)
			return nil
		}
		fmt.Print(doc)
		return nil
	},
}

func init() {
	instructionsCmd.Flags().StringVar(&instructionsClient, "client", "", "Render for a client: "+strings.Join(instructions.ClientNames(), ", "))
	instructionsCmd.Flags().StringVar(&instructionsOutput, "output", "", "Write the rendered document to this path instead of stdout")
	instructionsCmd.Flags().StringVarP(&instructionsScope, "scope", "s", "global", "Scope level for stored rules: global, project, agent, user")
	rootCmd.AddCommand(instructionsCmd)
}

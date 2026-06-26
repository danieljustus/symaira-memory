package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/spf13/cobra"
)

var (
	purgeDryRun     bool
	purgeSessionTTL string
	purgeScope      string
	purgeID         string
)

func init() {
	purgeCmd.Flags().BoolVar(&purgeDryRun, "dry-run", false, "Show what would be purged without deleting")
	purgeCmd.Flags().StringVar(&purgeSessionTTL, "session-ttl", "", "Purge session-scoped memories older than this (e.g. 24h, 7d)")
	purgeCmd.Flags().StringVar(&purgeScope, "scope", "", "Purge all non-archived memories in this scope")
	purgeCmd.Flags().StringVar(&purgeID, "id", "", "Purge a specific memory by ID")
	rootCmd.AddCommand(purgeCmd)
}

var purgeCmd = &cobra.Command{
	Use:   "purge",
	Short: "Purge expired or targeted memories (right-to-be-forgotten)",
	Long: `Removes memories based on TTL, scope, or specific ID.

Examples:
  # Purge session memories older than 24 hours
  symmemory purge --session-ttl 24h

  # Purge all project-scoped memories
  symmemory purge --scope project

  # Purge a specific memory
  symmemory purge --id <memory-id>

  # Preview what would be purged
  symmemory purge --session-ttl 24h --dry-run`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		db := GetDB()

		if purgeID != "" {
			exists, err := db.PurgeByID(purgeID)
			if err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "error purging memory")
			}
			if purgeDryRun {
				if exists {
					fmt.Fprintf(os.Stderr, "[dry-run] Would delete memory %s\n", purgeID)
				} else {
					fmt.Fprintf(os.Stderr, "Memory %s not found\n", purgeID)
				}
			} else if exists {
				fmt.Fprintf(os.Stderr, "Deleted memory %s\n", purgeID)
			} else {
				fmt.Fprintf(os.Stderr, "Memory %s not found\n", purgeID)
			}
			return nil
		}

		if purgeSessionTTL != "" {
			ttl, err := time.ParseDuration(purgeSessionTTL)
			if err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitNoInput, exitcodes.KindValidation, "invalid TTL duration")
			}
			if purgeDryRun {
				fmt.Fprintf(os.Stderr, "[dry-run] Would purge session memories older than %s\n", ttl)
			} else {
				n, err := db.PurgeExpiredMemories(ttl)
				if err != nil {
					return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "error purging expired memories")
				}
				fmt.Fprintf(os.Stderr, "Purged %d expired session memories\n", n)
			}
			return nil
		}

		if purgeScope != "" {
			if purgeDryRun {
				fmt.Fprintf(os.Stderr, "[dry-run] Would purge all memories in scope %q\n", purgeScope)
			} else {
				n, err := db.PurgeByScope(purgeScope)
				if err != nil {
					return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "error purging scope")
				}
				fmt.Fprintf(os.Stderr, "Purged %d memories in scope %q\n", n, purgeScope)
			}
			return nil
		}

		return exitcodes.Wrapf(nil, exitcodes.ExitNoInput, exitcodes.KindValidation, "no purge target specified; use --session-ttl, --scope, or --id")
	},
}

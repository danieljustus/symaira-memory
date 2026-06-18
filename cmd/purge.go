package cmd

import (
	"fmt"
	"os"
	"time"

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
	Run: func(cmd *cobra.Command, args []string) {
		db := GetDB()

		if purgeID != "" {
			exists, err := db.PurgeByID(purgeID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error purging memory: %v\n", err)
				os.Exit(1)
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
			return
		}

		if purgeSessionTTL != "" {
			ttl, err := time.ParseDuration(purgeSessionTTL)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Invalid TTL duration: %v\n", err)
				os.Exit(1)
			}
			if purgeDryRun {
				fmt.Fprintf(os.Stderr, "[dry-run] Would purge session memories older than %s\n", ttl)
			} else {
				n, err := db.PurgeExpiredMemories(ttl)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error purging expired memories: %v\n", err)
					os.Exit(1)
				}
				fmt.Fprintf(os.Stderr, "Purged %d expired session memories\n", n)
			}
			return
		}

		if purgeScope != "" {
			if purgeDryRun {
				fmt.Fprintf(os.Stderr, "[dry-run] Would purge all memories in scope %q\n", purgeScope)
			} else {
				n, err := db.PurgeByScope(purgeScope)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error purging scope: %v\n", err)
					os.Exit(1)
				}
				fmt.Fprintf(os.Stderr, "Purged %d memories in scope %q\n", n, purgeScope)
			}
			return
		}

		fmt.Fprintf(os.Stderr, "No purge target specified. Use --session-ttl, --scope, or --id.\n")
		os.Exit(1)
	},
}

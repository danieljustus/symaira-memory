package cmd

import (
	"fmt"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/spf13/cobra"
)

var (
	reviewPromote string
	reviewReject  string
	reviewLimit   int
)

func init() {
	reviewCmd.Flags().StringVar(&reviewPromote, "promote", "", "Approve the staged candidate with this memory ID so it becomes retrievable")
	reviewCmd.Flags().StringVar(&reviewReject, "reject", "", "Discard the staged candidate with this memory ID")
	reviewCmd.Flags().IntVar(&reviewLimit, "limit", 100, "Maximum number of candidates to list")
	rootCmd.AddCommand(reviewCmd)
}

var reviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Review staged candidate memories",
	Long: `Lists memories that were written as staged candidates (autonomous writes
held back from retrieval until a human confirms them) and lets you promote
or reject them.

Without flags the review queue is printed. Use --promote <id> to approve a
candidate (it becomes retrievable) or --reject <id> to discard it.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if reviewPromote != "" && reviewReject != "" {
			return exitcodes.Wrapf(fmt.Errorf("--promote and --reject are mutually exclusive"), exitcodes.ExitData, exitcodes.KindValidation, "review failed")
		}

		database := GetDB()
		if database == nil {
			return exitcodes.Wrapf(fmt.Errorf("database not available"), exitcodes.ExitSoftware, exitcodes.KindInternal, "review failed")
		}

		if reviewPromote != "" {
			if err := promoteCandidate(database, reviewPromote); err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "promote failed")
			}
			fmt.Printf("Memory %s promoted: it is now retrievable.\n", reviewPromote)
			return nil
		}
		if reviewReject != "" {
			if err := rejectCandidate(database, reviewReject); err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "reject failed")
			}
			fmt.Printf("Memory %s rejected and removed.\n", reviewReject)
			return nil
		}

		candidates, err := database.ListStagedMemories(reviewLimit)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "review failed")
		}
		if len(candidates) == 0 {
			fmt.Println("No staged candidates awaiting review.")
			return nil
		}
		fmt.Printf("%d staged candidate(s) awaiting review:\n\n", len(candidates))
		for _, c := range candidates {
			kind := c.Kind
			if kind == "" {
				kind = "unclassified"
			}
			content := c.Content
			if len(content) > 100 {
				content = content[:100] + "..."
			}
			fmt.Printf("  %s  [%s] %s\n      created: %s | scope: %s | id: %s\n",
				kind, c.Scope, content, c.CreatedAt.Format("2006-01-02 15:04"), c.Scope, c.ID)
		}
		fmt.Printf("\nPromote or reject with: symmemory review --promote <id> | --reject <id>\n")
		return nil
	},
}

// promoteCandidate approves a staged candidate from the CLI (audit-logged).
func promoteCandidate(database *db.DB, id string) error {
	m, err := database.GetMemory(id)
	if err != nil {
		return err
	}
	if m == nil {
		return fmt.Errorf("memory not found: %s", id)
	}
	if err := database.SetMemoryReviewStatus(id, db.ReviewApproved); err != nil {
		return err
	}
	_ = database.LogAudit("promote", id, m.Scope, m.CreatedSession, m.CreatedBy, "")
	return nil
}

// rejectCandidate discards a staged candidate from the CLI (audit-logged).
// Live memories are refused, never silently deleted.
func rejectCandidate(database *db.DB, id string) error {
	m, err := database.GetMemory(id)
	if err != nil {
		return err
	}
	if m == nil {
		return fmt.Errorf("memory not found: %s", id)
	}
	if m.ReviewStatus != db.ReviewStaged {
		return fmt.Errorf("memory %s is not a staged candidate (review_status=%s)", id, m.ReviewStatus)
	}
	_ = database.LogAudit("reject", id, m.Scope, m.CreatedSession, m.CreatedBy, "")
	return database.DeleteMemory(id)
}

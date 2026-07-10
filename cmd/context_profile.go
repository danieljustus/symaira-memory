package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var (
	cpBaseScope    string
	cpDescription  string
	cpParent       string
	cpOrder        int
	cpFilterKey    string
	cpFilterValue  string
)

var contextProfileCmd = &cobra.Command{
	Use:   "context-profile",
	Short: "Manage context profiles for inherited memory retrieval",
	Long: `Create, list, and manage context profiles that define ordered scope
inheritance chains. When searching with a profile, memories are retrieved
from multiple scopes in the profile's precedence order.`,
}

var cpAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Create a new context profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		cp := &db.ContextProfile{
			ID:          uuid.New().String(),
			Name:        name,
			Description: cpDescription,
			BaseScope:   cpBaseScope,
		}
		if err := GetDB().SaveContextProfile(cp); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to save context profile")
		}
		fmt.Fprintf(os.Stderr, "Context profile created: %s (base_scope=%s)\n", cp.Name, cp.BaseScope)
		return nil
	},
}

var cpListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all context profiles",
	RunE: func(cmd *cobra.Command, args []string) error {
		profiles, err := GetDB().ListContextProfiles()
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to list context profiles")
		}
		if len(profiles) == 0 {
			fmt.Fprintln(os.Stderr, "No context profiles configured.")
			return nil
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(profiles)
	},
}

var cpLinkCmd = &cobra.Command{
	Use:   "link <profile> <scope>",
	Short: "Add a scope link to a context profile",
	Long: `Add an ordered scope link to a context profile. The profile will search
this scope when resolved. Use --parent to inherit from another profile,
and --order to control precedence (lower = searched first).`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		profileName := args[0]
		scope := args[1]

		cp, err := GetDB().GetContextProfileByName(profileName)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to fetch context profile")
		}
		if cp == nil {
			return exitcodes.Wrapf(nil, exitcodes.ExitNotFound, exitcodes.KindNotFound, "context profile not found: %s", profileName)
		}

		var parentID *string
		if cpParent != "" {
			parent, perr := GetDB().GetContextProfileByName(cpParent)
			if perr != nil {
				return exitcodes.Wrapf(perr, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to fetch parent profile")
			}
			if parent == nil {
				return exitcodes.Wrapf(nil, exitcodes.ExitNotFound, exitcodes.KindNotFound, "parent context profile not found: %s", cpParent)
			}
			parentID = &parent.ID
		}

		order := cpOrder
		if order == 0 {
			existing, _ := GetDB().ListContextProfileLinks(profileName)
			order = len(existing) + 1
		}

		link := &db.ContextProfileLink{
			ProfileID:       cp.ID,
			ParentProfileID: parentID,
			Scope:           scope,
			FilterKey:       cpFilterKey,
			FilterValue:     cpFilterValue,
			PrecedenceOrder: order,
		}
		if err := GetDB().AddContextProfileLink(link); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to add link")
		}
		fmt.Fprintf(os.Stderr, "Link added: profile=%s scope=%s order=%d\n", profileName, scope, order)
		return nil
	},
}

var cpUnlinkCmd = &cobra.Command{
	Use:   "unlink <profile> [scope]",
	Short: "Remove a scope link from a context profile",
	Long: `Remove scope links from a context profile. When scope is provided,
only that link is removed. When scope is omitted, all links are removed.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		profileName := args[0]
		scope := ""
		if len(args) > 1 {
			scope = args[1]
		}
		if err := GetDB().RemoveContextProfileLink(profileName, scope); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to remove link")
		}
		if scope != "" {
			fmt.Fprintf(os.Stderr, "Link removed: profile=%s scope=%s\n", profileName, scope)
		} else {
			fmt.Fprintf(os.Stderr, "All links removed: profile=%s\n", profileName)
		}
		return nil
	},
}

var cpShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show the resolved scope chain for a context profile",
	Long: `Resolve a context profile into its full ordered scope chain, including
inherited scopes from parent profiles. Detects cycles and depth violations.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		cp, err := GetDB().GetContextProfileByName(name)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to fetch context profile")
		}
		if cp == nil {
			return exitcodes.Wrapf(nil, exitcodes.ExitNotFound, exitcodes.KindNotFound, "context profile not found: %s", name)
		}

		scopes, err := GetDB().ResolveContextProfile(name, db.DefaultMaxDepth)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to resolve context profile")
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "PROFILE\t%s\n", cp.Name)
		fmt.Fprintf(w, "DESCRIPTION\t%s\n", cp.Description)
		fmt.Fprintf(w, "BASE_SCOPE\t%s\n\n", cp.BaseScope)

		if len(scopes) == 0 {
			fmt.Fprintln(w, "(no scopes resolved)")
		} else {
			fmt.Fprintln(w, "ORDER\tSCOPE\tPROFILE\tFILTER")
			for i, s := range scopes {
				filter := ""
				if s.FilterKey != "" {
					filter = s.FilterKey + "=" + s.FilterValue
				}
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", i+1, s.Scope, s.Profile, filter)
			}
		}
		return w.Flush()
	},
}

var cpResolveCmd = &cobra.Command{
	Use:   "resolve <name>",
	Short: "Resolve and print scope chain as JSON",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		scopes, err := GetDB().ResolveContextProfile(name, db.DefaultMaxDepth)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to resolve context profile")
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(scopes)
	},
}

func init() {
	cpAddCmd.Flags().StringVar(&cpBaseScope, "base-scope", "", "Fallback scope: global, project, agent, user, session")
	cpAddCmd.Flags().StringVar(&cpDescription, "description", "", "Human-readable description")

	cpLinkCmd.Flags().StringVar(&cpParent, "parent", "", "Parent context profile name for inheritance")
	cpLinkCmd.Flags().IntVar(&cpOrder, "order", 0, "Precedence order (lower = searched first; 0 = auto)")
	cpLinkCmd.Flags().StringVar(&cpFilterKey, "filter-key", "", "Metadata key to filter memories by")
	cpLinkCmd.Flags().StringVar(&cpFilterValue, "filter-value", "", "Required when filter-key is set")

	contextProfileCmd.AddCommand(cpAddCmd)
	contextProfileCmd.AddCommand(cpListCmd)
	contextProfileCmd.AddCommand(cpLinkCmd)
	contextProfileCmd.AddCommand(cpUnlinkCmd)
	contextProfileCmd.AddCommand(cpShowCmd)
	contextProfileCmd.AddCommand(cpResolveCmd)
	rootCmd.AddCommand(contextProfileCmd)
}

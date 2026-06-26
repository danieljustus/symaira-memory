package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/security"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var (
	profileRole        string
	profileType        string
	profileDescription string
)

func init() {
	profileCmd.AddCommand(profileAddCmd)
	profileCmd.AddCommand(profileListCmd)
	profileCmd.AddCommand(profileSetRoleCmd)
	profileCmd.AddCommand(profileRemoveCmd)

	profileAddCmd.Flags().StringVar(&profileRole, "role", "readwrite", "Role: read, readwrite, admin")
	profileAddCmd.Flags().StringVar(&profileType, "type", "agent", "Profile type: agent, human")
	profileAddCmd.Flags().StringVar(&profileDescription, "description", "", "Human-readable description")

	rootCmd.AddCommand(profileCmd)
}

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage agent profiles and role-based permissions",
	Long:  `Create, list, and manage agent profiles that control which clients can read or write memories.`,
}

var profileAddCmd = &cobra.Command{
	Use:   "add [name]",
	Short: "Create a new agent profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		role := string(security.ParseRole(profileRole))

		p := &db.Profile{
			ID:          uuid.New().String(),
			Name:        name,
			Type:        profileType,
			Role:        role,
			Description: profileDescription,
			Metadata:    map[string]any{},
		}

		if err := GetDB().SaveProfile(p); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to save profile")
		}

		fmt.Printf("Profile created: %s (role=%s, type=%s)\n", p.Name, p.Role, p.Type)
		return nil
	},
}

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all agent profiles",
	RunE: func(cmd *cobra.Command, args []string) error {
		profiles, err := GetDB().ListProfiles()
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to list profiles")
		}

		if len(profiles) == 0 {
			fmt.Println("No profiles configured.")
			return nil
		}

		bytes, err := json.MarshalIndent(profiles, "", "  ")
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "error encoding profiles")
		}
		fmt.Println(string(bytes))
		return nil
	},
}

var profileSetRoleCmd = &cobra.Command{
	Use:   "set-role [name] [role]",
	Short: "Update the role of an existing profile",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		newRole := string(security.ParseRole(args[1]))

		p, err := GetDB().GetProfileByName(name)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to fetch profile")
		}
		if p == nil {
			return exitcodes.Wrapf(nil, exitcodes.ExitNotFound, exitcodes.KindNotFound, "profile not found: %s", name)
		}

		p.Role = newRole
		if err := GetDB().SaveProfile(p); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to update profile")
		}

		fmt.Printf("Profile %q role updated to %s\n", p.Name, p.Role)
		return nil
	},
}

var profileRemoveCmd = &cobra.Command{
	Use:   "remove [name]",
	Short: "Delete an agent profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if err := GetDB().DeleteProfile(name); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to delete profile")
		}
		fmt.Printf("Profile %q removed.\n", name)
		return nil
	},
}

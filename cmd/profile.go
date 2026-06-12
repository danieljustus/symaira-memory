package cmd

import (
	"encoding/json"
	"fmt"
	"os"

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
	Run: func(cmd *cobra.Command, args []string) {
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
			fmt.Fprintf(os.Stderr, "Error saving profile: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Profile created: %s (role=%s, type=%s)\n", p.Name, p.Role, p.Type)
	},
}

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all agent profiles",
	Run: func(cmd *cobra.Command, args []string) {
		profiles, err := GetDB().ListProfiles()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing profiles: %v\n", err)
			os.Exit(1)
		}

		if len(profiles) == 0 {
			fmt.Println("No profiles configured.")
			return
		}

		bytes, err := json.MarshalIndent(profiles, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding profiles: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(bytes))
	},
}

var profileSetRoleCmd = &cobra.Command{
	Use:   "set-role [name] [role]",
	Short: "Update the role of an existing profile",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		newRole := string(security.ParseRole(args[1]))

		p, err := GetDB().GetProfileByName(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching profile: %v\n", err)
			os.Exit(1)
		}
		if p == nil {
			fmt.Fprintf(os.Stderr, "Profile not found: %s\n", name)
			os.Exit(1)
		}

		p.Role = newRole
		if err := GetDB().SaveProfile(p); err != nil {
			fmt.Fprintf(os.Stderr, "Error updating profile: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Profile %q role updated to %s\n", p.Name, p.Role)
	},
}

var profileRemoveCmd = &cobra.Command{
	Use:   "remove [name]",
	Short: "Delete an agent profile",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		if err := GetDB().DeleteProfile(name); err != nil {
			fmt.Fprintf(os.Stderr, "Error deleting profile: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Profile %q removed.\n", name)
	},
}

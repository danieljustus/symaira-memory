package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var (
	entityType        string
	entityAliases     string
	entityDescription string
)

func init() {
	entityCmd.AddCommand(entityAddCmd)
	entityCmd.AddCommand(entityListCmd)
	entityCmd.AddCommand(entityLinkCmd)
	entityCmd.AddCommand(entityShowCmd)
	entityCmd.AddCommand(entityRemoveCmd)

	entityAddCmd.Flags().StringVar(&entityType, "type", "other", "Entity type: person, project, org, other")
	entityAddCmd.Flags().StringVar(&entityAliases, "aliases", "", "Comma-separated aliases")
	entityAddCmd.Flags().StringVar(&entityDescription, "description", "", "Entity description")

	rootCmd.AddCommand(entityCmd)
}

var entityCmd = &cobra.Command{
	Use:   "entity",
	Short: "Manage entities (people, projects, organizations) for cross-memory linking",
}

var entityAddCmd = &cobra.Command{
	Use:   "add [name]",
	Short: "Create a new entity",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		var aliases []string
		if entityAliases != "" {
			for _, a := range strings.Split(entityAliases, ",") {
				a = strings.TrimSpace(a)
				if a != "" {
					aliases = append(aliases, a)
				}
			}
		}

		e := &db.Entity{
			ID:          uuid.New().String(),
			Name:        name,
			Type:        entityType,
			Aliases:     aliases,
			Description: entityDescription,
			CreatedBy:   "cli",
			CreatedAt:   time.Now().UTC(),
		}

		if err := GetDB().SaveEntity(e); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving entity: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Entity created: %s (type=%s)\n", e.Name, e.Type)
		if len(aliases) > 0 {
			fmt.Printf("  Aliases: %s\n", strings.Join(aliases, ", "))
		}
	},
}

var entityListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all entities",
	Run: func(cmd *cobra.Command, args []string) {
		entities, err := GetDB().ListEntities()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing entities: %v\n", err)
			os.Exit(1)
		}

		if len(entities) == 0 {
			fmt.Println("No entities found.")
			return
		}

		bytes, err := json.MarshalIndent(entities, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding entities: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(bytes))
	},
}

var entityLinkCmd = &cobra.Command{
	Use:   "link [memory-id] [entity-name]",
	Short: "Link a memory to an entity",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		memoryID := args[0]
		entityName := args[1]

		entity, err := GetDB().ResolveEntity(entityName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving entity: %v\n", err)
			os.Exit(1)
		}
		if entity == nil {
			fmt.Fprintf(os.Stderr, "Entity not found: %s\n", entityName)
			os.Exit(1)
		}

		m, err := GetDB().GetMemory(memoryID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching memory: %v\n", err)
			os.Exit(1)
		}
		if m == nil {
			fmt.Fprintf(os.Stderr, "Memory not found: %s\n", memoryID)
			os.Exit(1)
		}

		if err := GetDB().LinkMemoryToEntity(memoryID, entity.ID); err != nil {
			fmt.Fprintf(os.Stderr, "Error linking: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Linked memory %s to entity %q\n", memoryID, entity.Name)
	},
}

var entityShowCmd = &cobra.Command{
	Use:   "show [name]",
	Short: "Show entity details and linked memories",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		entity, err := GetDB().ResolveEntity(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving entity: %v\n", err)
			os.Exit(1)
		}
		if entity == nil {
			fmt.Fprintf(os.Stderr, "Entity not found: %s\n", name)
			os.Exit(1)
		}

		bytes, err := json.MarshalIndent(entity, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding entity: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(bytes))

		memoryIDs, err := GetDB().MemoryIDsForEntity(entity.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching linked memories: %v\n", err)
			os.Exit(1)
		}

		if len(memoryIDs) == 0 {
			fmt.Println("\nNo linked memories.")
			return
		}

		fmt.Printf("\nLinked memories (%d):\n", len(memoryIDs))
		for _, mid := range memoryIDs {
			m, err := GetDB().GetMemory(mid)
			if err != nil || m == nil {
				fmt.Printf("  - %s (error fetching)\n", mid)
				continue
			}
			content := m.Content
			if len(content) > 80 {
				content = content[:77] + "..."
			}
			fmt.Printf("  - %s: %s\n", m.ID, content)
		}
	},
}

var entityRemoveCmd = &cobra.Command{
	Use:   "remove [name]",
	Short: "Delete an entity and its memory links",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		entity, err := GetDB().ResolveEntity(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving entity: %v\n", err)
			os.Exit(1)
		}
		if entity == nil {
			fmt.Fprintf(os.Stderr, "Entity not found: %s\n", name)
			os.Exit(1)
		}

		if err := GetDB().DeleteEntity(entity.ID); err != nil {
			fmt.Fprintf(os.Stderr, "Error deleting entity: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Entity %q removed.\n", entity.Name)
	},
}

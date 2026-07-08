package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var (
	entityType           string
	entityAliases        string
	entityDescription    string
	entityNeighborsDepth int
)

func init() {
	entityCmd.AddCommand(entityAddCmd)
	entityCmd.AddCommand(entityListCmd)
	entityCmd.AddCommand(entityLinkCmd)
	entityCmd.AddCommand(entityShowCmd)
	entityCmd.AddCommand(entityRemoveCmd)
	entityCmd.AddCommand(entityRelateCmd)
	entityCmd.AddCommand(entityUnrelateCmd)
	entityCmd.AddCommand(entityNeighborsCmd)

	entityAddCmd.Flags().StringVar(&entityType, "type", "other", "Entity type: person, project, org, other")
	entityAddCmd.Flags().StringVar(&entityAliases, "aliases", "", "Comma-separated aliases")
	entityAddCmd.Flags().StringVar(&entityDescription, "description", "", "Entity description")
	entityNeighborsCmd.Flags().IntVar(&entityNeighborsDepth, "depth", 1, fmt.Sprintf("Traversal depth, 1-%d", db.MaxGraphDepth))

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
	RunE: func(cmd *cobra.Command, args []string) error {
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
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to save entity")
		}

		fmt.Printf("Entity created: %s (type=%s)\n", e.Name, e.Type)
		if len(aliases) > 0 {
			fmt.Printf("  Aliases: %s\n", strings.Join(aliases, ", "))
		}
		return nil
	},
}

var entityListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all entities",
	RunE: func(cmd *cobra.Command, args []string) error {
		entities, err := GetDB().ListEntities()
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to list entities")
		}

		formatter := NewOutputFormatter(GetOutputFormat(cmd))
		if err := formatter.Output(entities, "entity-list"); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "output error")
		}
		return nil
	},
}

var entityLinkCmd = &cobra.Command{
	Use:   "link [memory-id] [entity-name]",
	Short: "Link a memory to an entity",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		memoryID := args[0]
		entityName := args[1]

		entity, err := GetDB().ResolveEntity(entityName)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to resolve entity")
		}
		if entity == nil {
			return exitcodes.Wrapf(nil, exitcodes.ExitNotFound, exitcodes.KindNotFound, "entity not found: %s", entityName)
		}

		m, err := GetDB().GetMemory(memoryID)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to fetch memory")
		}
		if m == nil {
			return exitcodes.Wrapf(nil, exitcodes.ExitNotFound, exitcodes.KindNotFound, "memory not found: %s", memoryID)
		}

		if err := GetDB().LinkMemoryToEntity(memoryID, entity.ID); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to link memory to entity")
		}

		fmt.Printf("Linked memory %s to entity %q\n", memoryID, entity.Name)
		return nil
	},
}

var entityShowCmd = &cobra.Command{
	Use:   "show [name]",
	Short: "Show entity details and linked memories",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		entity, err := GetDB().ResolveEntity(name)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to resolve entity")
		}
		if entity == nil {
			return exitcodes.Wrapf(nil, exitcodes.ExitNotFound, exitcodes.KindNotFound, "entity not found: %s", name)
		}

		bytes, err := json.MarshalIndent(entity, "", "  ")
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "error encoding entity")
		}
		fmt.Println(string(bytes))

		outRels, err := GetDB().OutgoingRelations(entity.ID)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "error fetching outgoing relations")
		}
		inRels, err := GetDB().IncomingRelations(entity.ID)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "error fetching incoming relations")
		}
		if len(outRels) > 0 || len(inRels) > 0 {
			fmt.Println("\nRelations:")
			for _, r := range outRels {
				fmt.Printf("  --%s--> %s\n", r.RelationType, r.ToEntityID)
			}
			for _, r := range inRels {
				fmt.Printf("  <--%s-- %s\n", r.RelationType, r.FromEntityID)
			}
		}

		memoryIDs, err := GetDB().MemoryIDsForEntity(entity.ID)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "error fetching linked memories")
		}

		if len(memoryIDs) == 0 {
			fmt.Println("\nNo linked memories.")
			return nil
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
		return nil
	},
}

var entityRelateCmd = &cobra.Command{
	Use:   "relate [from] [relation] [to]",
	Short: "Create a directed relation between two entities",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		from, relation, to := args[0], args[1], args[2]

		fromEntity, err := GetDB().ResolveEntity(from)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to resolve entity")
		}
		if fromEntity == nil {
			return exitcodes.Wrapf(nil, exitcodes.ExitNotFound, exitcodes.KindNotFound, "entity not found: %s", from)
		}

		toEntity, err := GetDB().ResolveEntity(to)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to resolve entity")
		}
		if toEntity == nil {
			return exitcodes.Wrapf(nil, exitcodes.ExitNotFound, exitcodes.KindNotFound, "entity not found: %s", to)
		}

		rel := &db.EntityRelation{
			FromEntityID: fromEntity.ID,
			ToEntityID:   toEntity.ID,
			RelationType: relation,
			CreatedBy:    "cli",
			CreatedAt:    time.Now().UTC(),
		}
		if err := GetDB().SaveEntityRelation(rel); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to save relation")
		}

		fmt.Printf("Related: %s --%s--> %s\n", fromEntity.Name, relation, toEntity.Name)
		return nil
	},
}

var entityUnrelateCmd = &cobra.Command{
	Use:   "unrelate [from] [relation] [to]",
	Short: "Remove a directed relation between two entities",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		from, relation, to := args[0], args[1], args[2]

		fromEntity, err := GetDB().ResolveEntity(from)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to resolve entity")
		}
		if fromEntity == nil {
			return exitcodes.Wrapf(nil, exitcodes.ExitNotFound, exitcodes.KindNotFound, "entity not found: %s", from)
		}

		toEntity, err := GetDB().ResolveEntity(to)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to resolve entity")
		}
		if toEntity == nil {
			return exitcodes.Wrapf(nil, exitcodes.ExitNotFound, exitcodes.KindNotFound, "entity not found: %s", to)
		}

		if err := GetDB().DeleteEntityRelation(fromEntity.ID, toEntity.ID, relation); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to delete relation")
		}

		fmt.Printf("Unrelated: %s --%s--> %s\n", fromEntity.Name, relation, toEntity.Name)
		return nil
	},
}

// entityNeighborsResult is the shared shape returned by the CLI's --output
// json form and the graph_neighbors MCP tool.
type entityNeighborsResult struct {
	Nodes []*db.Entity         `json:"nodes"`
	Edges []*db.EntityRelation `json:"edges"`
}

var entityNeighborsCmd = &cobra.Command{
	Use:   "neighbors [name]",
	Short: "Show the entities and relations reachable from an entity",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		entity, err := GetDB().ResolveEntity(name)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to resolve entity")
		}
		if entity == nil {
			return exitcodes.Wrapf(nil, exitcodes.ExitNotFound, exitcodes.KindNotFound, "entity not found: %s", name)
		}

		nodes, edges, err := GetDB().GraphNeighbors(entity.ID, entityNeighborsDepth)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to compute neighbors")
		}

		if GetOutputFormat(cmd) == "json" {
			formatter := NewOutputFormatter(GetOutputFormat(cmd))
			return formatter.Output(entityNeighborsResult{Nodes: nodes, Edges: edges}, "entity-neighbors")
		}

		fmt.Printf("Nodes (%d):\n", len(nodes))
		for _, n := range nodes {
			fmt.Printf("  - %s (%s)\n", n.Name, n.Type)
		}
		fmt.Printf("\nEdges (%d):\n", len(edges))
		for _, e := range edges {
			fmt.Printf("  %s --%s--> %s\n", e.FromEntityID, e.RelationType, e.ToEntityID)
		}
		return nil
	},
}

var entityRemoveCmd = &cobra.Command{
	Use:   "remove [name]",
	Short: "Delete an entity and its memory links",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		entity, err := GetDB().ResolveEntity(name)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to resolve entity")
		}
		if entity == nil {
			return exitcodes.Wrapf(nil, exitcodes.ExitNotFound, exitcodes.KindNotFound, "entity not found: %s", name)
		}

		if err := GetDB().DeleteEntity(entity.ID); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to delete entity")
		}

		fmt.Printf("Entity %q removed.\n", entity.Name)
		return nil
	},
}

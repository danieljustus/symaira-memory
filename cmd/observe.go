package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/danieljustus/symaira-memory/internal/memory"
	"github.com/spf13/cobra"
)

// observeCmd is the parent command for recording session-scoped hook events.
// It is the write-path companion to the context command: while context reads
// and assembles the memory store, observe writes events from agent lifecycle
// hooks into the store. All events are PII-guarded before storage.
// Fail-safe: every subcommand always exits 0 and never writes to stdout.
var observeCmd = &cobra.Command{
	Use:   "observe",
	Short: "Record session-scoped hook events (write-path)",
	Long: `Record session-scoped events from agent lifecycle hooks into the
memory store. All events are PII-guarded before storage. This is the write-path
companion to the context command, designed to be invoked by agent hooks such as
Claude Code's PostToolUseFailure, SessionEnd, and PreCompact lifecycle events.

Fail-safe: always exits 0 and never writes to stdout — output and diagnostics
go to stderr. Without a working config or database the command silently records
nothing and exits successfully.`,

	// PersistentPreRunE: observe children load config/DB themselves; never
	// inherit the root command's database initialization.
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

// --------------------------------------------------------------------------
// observe tool-failure: record a tool failure event from agent hooks
// --------------------------------------------------------------------------

var observeToolFailureCmd = &cobra.Command{
	Use:   "tool-failure",
	Short: "Record a PostToolUseFailure hook event",
	Long: `Record a tool failure event from agent lifecycle hooks into the
memory store as a session-scoped working memory. High-signal events like tool
failures are captured so the agent's next session has context about what went
wrong. Content is PII-guarded before storage.

Fail-safe: always exits 0, never writes to stdout.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		data, _ := cmd.Flags().GetString("data")
		handleObserveEvent("PostToolUseFailure", data)
		return nil
	},
}

// --------------------------------------------------------------------------
// observe session-end: record a SessionEnd hook event
// --------------------------------------------------------------------------

var observeSessionEndCmd = &cobra.Command{
	Use:   "session-end",
	Short: "Record a SessionEnd hook event",
	Long: `Record a session-end event from agent lifecycle hooks. This allows
the agent to capture session duration and context for future sessions.

Fail-safe: always exits 0, never writes to stdout.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		data, _ := cmd.Flags().GetString("data")
		handleObserveEvent("SessionEnd", data)
		return nil
	},
}

// --------------------------------------------------------------------------
// observe pre-compact: record a PreCompact hook event
// --------------------------------------------------------------------------

var observePreCompactCmd = &cobra.Command{
	Use:   "pre-compact",
	Short: "Record a PreCompact hook event",
	Long: `Record a pre-compact event from agent lifecycle hooks. Signals that
the agent's conversation history is about to be compacted.

Fail-safe: always exits 0, never writes to stdout.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		data, _ := cmd.Flags().GetString("data")
		handleObserveEvent("PreCompact", data)
		return nil
	},
}

// --------------------------------------------------------------------------
// Shared handler
// --------------------------------------------------------------------------

// handleObserveEvent loads config and DB, stores a session-scoped working
// memory for the given hook event type. All output goes to stderr; the
// function always returns nil (fail-safe) and never writes to stdout.
func handleObserveEvent(eventType, data string) {
	// Fail-safe: load config, silently return on error
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "observe %s: config load failed: %v\n", eventType, err)
		return
	}

	// Fail-safe: open database, silently return on error
	database, err := db.Open(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "observe %s: db open failed: %v\n", eventType, err)
		return
	}
	defer database.Close()

	// Build event content
	content := fmt.Sprintf("Agent hook event: %s", eventType)
	if data != "" {
		content = fmt.Sprintf("Agent hook event: %s — %s", eventType, data)
	}

	// Metadata carries event provenance
	meta := map[string]string{
		"source_type": "direct",
		"source_tool": "hook:claude-code",
		"hook_event":  eventType,
		"observed_at": time.Now().UTC().Format(time.RFC3339),
	}

	embeddings := extractor.NewEmbeddingsGenerator(cfg)
	patternExtractor := extractor.NewPatternExtractor()

	_, _, err = memory.Store(
		database,
		embeddings,
		patternExtractor,
		content,
		"session", // Scope: session-scoped
		meta,
		true, // PII guard enabled
		memory.Attribution{
			Author:    "hook:claude-code",
			SessionID: "",
		},
		nil,         // No entity linking
		"hook:claude-code",
		true,                // Working memory (TTL-based eviction)
		24*time.Hour,        // Default TTL for hook events
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "observe %s: store failed: %v\n", eventType, err)
		return
	}
}

// --------------------------------------------------------------------------
// Registration
// --------------------------------------------------------------------------

func init() {
	// Flags
	observeToolFailureCmd.Flags().String("data", "", "Optional JSON event payload (PII-guarded before storage)")
	observeSessionEndCmd.Flags().String("data", "", "Optional JSON event payload")
	observePreCompactCmd.Flags().String("data", "", "Optional JSON event payload")

	// Wire subcommands
	observeCmd.AddCommand(observeToolFailureCmd)
	observeCmd.AddCommand(observeSessionEndCmd)
	observeCmd.AddCommand(observePreCompactCmd)

	// Register on root
	rootCmd.AddCommand(observeCmd)
}

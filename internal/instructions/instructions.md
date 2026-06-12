# Symaira Memory — Agent Integration Guide

Symaira Memory (`symmemory`) is a persistent semantic database for developer preferences, codebase guidelines, and session context. It exposes four MCP tools over stdio JSON-RPC 2.0.

## Tools

| Tool | Purpose |
|---|---|
| `memory_search` | Semantic vector similarity search across stored memories |
| `memory_set` | Store a new persistent memory or fact |
| `memory_get` | Retrieve a specific memory by its UUID |
| `memory_list` | List all memories, optionally filtered by scope |

## When to Use `memory_search`

- **Session start**: At the beginning of every session or task, search for relevant context using key terms (e.g., "code style", "database settings", "language preference").
- **Before decisions**: When about to make an architectural or design choice, search for prior decisions to avoid contradicting established patterns.
- **Fact lookup**: When the user references something that may have been discussed before, search to retrieve the original context.

## When to Use `memory_set`

Autonomously store memories when the user expresses persistent information. Do not ask for permission.

**Store these:**
- User preferences: "User prefers TypeScript for scripting tasks."
- Project rules: "API daemon runs on port 8787."
- Architectural decisions: "Shared services must remain free of private commercial logic."
- Constraints: "No CGO dependencies in the build."

**Do NOT store:**
- Temporary debug states ("Fixed bug on line 42").
- Large raw logs, code blocks, or error tracebacks.
- Conversational filler.

## Scope Selection

| Scope | When to use |
|---|---|
| `project` | **Default choice.** Any fact specific to the current codebase or repository. Auto-resolves project name from `.symmemory.toml` or `.git` in parent directories. |
| `global` | General user preferences that apply across all projects (e.g., "User prefers tabs over spaces"). |
| `agent` | Facts specific to a particular AI agent or tool. |
| `user` | Facts tied to a specific human user identity. |
| `session` | Ephemeral facts relevant only to the current conversation. |

## Entity Tagging

Mention relevant entity names (people, services, projects) directly in the memory content so they become searchable. For example: "The **auth-service** uses JWT tokens signed by **symmemory**."

## Provenance Tracking

Pass `session_id` when calling `memory_set` to record which conversation session created the memory. This enables auditing and traceability.

## Fact Formatting

Write facts as objective, third-person, declarative statements:
- **Good**: "User prefers tabs over spaces in Go source files."
- **Bad**: "I need to use tabs."
- **Good**: "Project backend uses SQLite with WAL mode."
- **Bad**: "We enabled WAL mode."

## Memory Consolidation

When the user updates a prior decision (e.g., "Switch from SQLite to PostgreSQL"):
1. Run `memory_search` to find the stale memory.
2. Note the old memory ID so it can be deleted or updated.

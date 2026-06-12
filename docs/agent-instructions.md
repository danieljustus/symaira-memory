# AI Agent Instructions

The canonical agent integration guide is embedded in the `symmemory` binary.

## Access

```bash
# Print the full guide to stdout
symmemory instructions

# The same text is returned in the MCP `initialize` response
# under the "instructions" field
```

## MCP Initialize Response

When an MCP client sends the `initialize` handshake, the server returns the full
agent instructions in the `instructions` field of the result object. No separate
file or URL is needed — the binary is the source of truth.

## Skill File

A ready-to-use skill definition lives at `skills/symmemory/SKILL.md` in the
repository. Copy it to your agent's skill directory (e.g., `~/.claude/skills/`)
to enable automatic tool discovery.

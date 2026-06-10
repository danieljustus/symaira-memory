# Symaira Memory (symaira-memory)

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

> **Symaira Memory** is a next-generation persistent memory layer, context synchronization engine, and semantic knowledge base built for the **Human-AI Symbiosis Era**.
>
> **Repository**: `symaira-memory`
> **CLI Command**: `symmemory` (analogous to `symaira-vault` and its CLI `symvault`)

It enables seamless, long-term memory sharing and contextual continuity between humans and their AI counterparts, ensuring intelligence is aligned, retrieved, and expanded across platforms.

---

## The Vision

In the Human-AI Symbiosis Era, the bottleneck of productivity is no longer compute, but *shared context*. AI agents need to remember past interactions, learn user preferences dynamically, and possess a persistent semantic memory layer that spans across different apps, platforms, and devices.

**Symaira Memory** (`symmemory`) provides the unified infrastructure to make this possible.

---

## Features

- **Persistent local SQLite storage**: All memories store in WAL-mode SQLite under standard XDG paths (`~/.local/share/symmemory/`). No external databases required.
- **Hybrid semantic search**: Two-layer embedding pipeline. Tries Ollama (`nomic-embed-text`) first for real vector embeddings, falls back to a deterministic word-hash vectorizer (FNV-1a) when Ollama is offline. Search uses pure Go cosine similarity with zero CGO.
- **Model Context Protocol (MCP) server**: Speak the MCP stdio JSON-RPC 2.0 protocol natively. Plug into Claude Desktop, Cursor, VS Code (Cline / Roo Code / Continue.dev), and any MCP-compatible host.
- **HTTP REST API daemon**: Run `symmemory serve -p 8787` for browser extensions, dashboards, and remote clients. Protected by JWT authentication.
- **Web Console**: Built-in browser dashboard served at `http://localhost:8787/` when running `symmemory serve`. Browse, search, and delete memories with a clean UI. No npm, no frameworks, no CDN — fully offline.
- **Browser extension**: Chrome/Edge/Brave Manifest V3 extension injects memory context into ChatGPT, Claude Web, and Perplexity. Ships in `extension/`.
- **TUI dashboard**: Terminal-based memory browser and curator built with Bubble Tea and Lip Gloss. Launch with `symmemory console`.
- **PII Guard**: Automatic regex-based redaction of credit cards, email addresses, and API keys before anything touches disk.
- **JWT authentication**: Generate and verify signed tokens for REST API access. HMAC-SHA256, configurable expiry and subject.
- **Memory scoping**: Organize memories by scope (global, project, agent, user, session). Project scope auto-detects `.git` or `.symmemory.toml` in parent directories.
- **Behavioral rules**: Store procedural instructions for AI agents, automatically injected into prompts. Manage with `symmemory rule`.
- **Encrypted backup / restore**: Export your SQLite database to compressed `.tar.gz` archives with optional AES-256-GCM encryption.
- **Extractive dialogue summarizer**: Reduce LLM context cost by 60-70% via keyword-weighted sentence extraction.
- **Zero CGO**: Pure Go compilation. Builds on any platform without C toolchains. Uses `modernc.org/sqlite` instead of `mattn/go-sqlite3`.

---

## Installation

### Prerequisites

- Go 1.26.3 or later
- No C compiler required (CGO-free)

### From source (go install)

```bash
go install github.com/danieljustus/symaira-memory@latest
```

### From source (build manually)

```bash
git clone https://github.com/danieljustus/symaira-memory.git
cd symaira-memory
go build -o symmemory main.go
./symmemory version
```

---

## Quickstart

```bash
# Save a fact
symmemory set --value "Alice prefers dark mode in all applications." --scope global

# List all stored memories
symmemory list

# Search semantically by relevance
symmemory search "preferred theme settings" --limit 5

# Retrieve a single memory by its ID
symmemory get <memory-id>

# Launch the interactive TUI browser
symmemory console

# Start the MCP server (for agent integration)
symmemory serve

# Generate an API token for HTTP access
symmemory token generate --subject "my-agent" --duration 720
```

For a full reference of all commands and flags, run `symmemory --help`.

---

## Agent Integration

Symaira Memory speaks the Model Context Protocol (MCP) natively. AI agents connect over stdio JSON-RPC 2.0 and gain four tools: `memory_get`, `memory_set`, `memory_search`, and `memory_list`.

Run `symmemory mcp-config` to print ready-to-paste configuration blocks for Claude Desktop, Cursor, and VS Code. For detailed setup guides covering each host, browser extension installation, and optimal agent system prompts, see [docs/agent-integration.md](docs/agent-integration.md).

---

## Web Console

Start the HTTP daemon and open the built-in dashboard:

```bash
# Generate a token
TOKEN=$(symmemory token generate --subject "console" --duration 720)

# Start the server
symmemory serve -p 8787

# Open http://localhost:8787 in your browser
# Paste the token in the console to authenticate
```

The Web Console provides:

- **Memory browser**: List all memories with scope filtering (global/project/user/agent/session)
- **Semantic search**: Query memories by natural language with relevance scores
- **Delete management**: Remove memories with confirmation
- **Rules viewer**: Read-only list of behavioral rules
- **Status monitoring**: Real-time connection status

The dashboard is embedded in the binary via `//go:embed` — no external dependencies, no build step, works fully offline.

---

## Configuration

Settings live in `~/.config/symmemory/config.toml`. Run `symmemory config init` to scaffold a file with all supported fields and their defaults.

Environment variables:

| Variable | Default | Purpose |
| :--- | :--- | :--- |
| `SYMMEMORY_DB_PATH` | XDG default | Override the SQLite database path |
| `OLLAMA_API_URL` | `http://localhost:11434/api/embeddings` | Embedding endpoint |
| `OLLAMA_MODEL` | `nomic-embed-text` | Embedding model |
| `OPENAI_API_KEY` | none | Cloud LLM fact cleaning fallback |
| `JWT_SECRET_KEY` | auto-generated | Token signing secret |

Per-project scoping is configured with a `.symmemory.toml` file in your project root. See [docs/configuration.md](docs/configuration.md) for details.

---

## Security & Privacy

- **PII Guard**: All memory content passes through a regex filter that redacts credit cards, email addresses, and API tokens before storage.
- **JWT Auth**: REST API endpoints require signed bearer tokens. Tokens are scoped to named subjects with configurable expiration.
- **Encrypted backups**: Backup archives can be encrypted with AES-256-GCM using a password you provide. Decryption requires the same password.
- **Local-first**: The database stays on your machine under `~/.local/share/symmemory/`. No telemetry, no external calls (Ollama is optional and local).
- **Scope isolation**: Memories are isolated by project, agent, user, and session boundaries. Agents only see what their scope permits.

---

## Roadmap

1. **Phase 1**: Repository setup and architecture layout
2. **Phase 2**: Local memory core and SQLite/Vector storage implementation
3. **Phase 3**: Model Context Protocol (MCP) Server support and HTTP REST API
4. **Phase 4**: Multi-device sync and encrypted cloud backup (Symaira Memory Pro) *Next*
5. **Phase 5**: Web-based Memory Console (Dashboard)

Phases 1 through 3 and Phase 5 are implemented and shipped. Phase 4 is in planning.

---

## Architecture

For a deep dive into the data pipeline, component design, and scope isolation model, see [docs/architecture.md](docs/architecture.md).

---

Copyright &copy; 2026 Daniel Justus. All rights reserved. Licensed under the MIT License.

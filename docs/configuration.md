# Configuration Reference

Symaira Memory is local-first by design. Settings are configured globally or per active directory project workspace.

---

## 📂 Active Workspace Scoping (`.symmemory.toml`)

To isolate memories to a specific project (so that your AI agent only retrieves facts related to the current codebase), create a `.symmemory.toml` configuration file in the project's root folder:

```toml
# .symmemory.toml - Local project configuration
[project]
name = "my-awesome-app"
description = "Core SaaS repository"

[memory]
default_scope = "project"
token_budget = 2000
```

When you save memories using the `--scope project` flag, `symmemory` looks up your parent directories to detect the active project name, binding the memory database to that project.

---

## 🛠️ Global Settings

Global configurations are stored under standard XDG paths (e.g. `~/.config/symmemory/config.toml` or loadable via local environment parameters).

### Environment Variables

Configure these settings inside your shell configuration (`.zshrc` or `.bashrc`):

- `SYMMEMORY_DB_PATH` — Overrides the default XDG path for the SQLite database.
- `OLLAMA_API_URL` — Overrides the local Ollama embeddings url (default: `http://localhost:11434/api/embeddings`).
- `OLLAMA_MODEL` — Overrides the default embedding model (default: `nomic-embed-text`).
- `OPENAI_API_KEY` — If provided, enables cloud-fallback LLM fact cleaning and consolidation.
- `JWT_SECRET_KEY` — Overrides the token signing secret for HTTP daemon verification.

### Consolidation Settings

Configure the memory consolidation engine (dreaming) in `~/.config/symmemory/config.toml`:

```toml
[consolidation]
enabled = true
schedule = "0 2 * * *"          # Cron schedule for automatic consolidation
idle_timeout = "30m"             # How long to wait before consolidating idle memories
provider = "ollama"              # LLM provider: "ollama" or "openai"
model = "llama3"                 # Model name (e.g., "llama3", "gpt-4o-mini")
url = "http://localhost:11434/api/generate"  # LLM API endpoint URL
```

If `consolidation.url` is not set, it falls back to the Ollama URL from `[ollama]` config. If `consolidation.model` is not set, it falls back to the Ollama model. This allows consolidation to use a different LLM endpoint than the embeddings pipeline.

### Security Settings

Configure HTTP daemon access control in `~/.config/symmemory/config.toml`:

```toml
[security]
pii_enabled = true              # Redact PII from stored memory content (default: true)
trusted_proxies = []             # CIDR ranges trusted to set client-IP headers (default: none)
require_profile = false          # Deny write access to JWT subjects with no stored profile (default: false)
```

`require_profile` controls what happens when a valid JWT's subject (`--subject` passed to `symmemory token generate`) has no matching profile saved via `symmemory profile` (or `SaveProfile`):

- `false` (default): the request keeps default access, but a warning is logged (`JWT subject has no matching profile`). This preserves the existing behavior for setups that generate ad hoc tokens without maintaining a profile per subject.
- `true`: write endpoints (`memory_set`, `delete`, `sync/apply`) are denied with a 403 for subjects without a stored profile. Read access is unaffected. A profile lookup failure (a real database error, not merely "not found") is always denied regardless of this setting — role enforcement fails closed rather than silently granting full access.

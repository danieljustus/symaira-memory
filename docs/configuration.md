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

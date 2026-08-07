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

### MCP Settings

Configure MCP stdio server attribution in `~/.config/symmemory/config.toml`:

```toml
[mcp]
client_id = ""   # Fixed attribution identity for MCP writes (created_by/updated_by).
                 # When set, it wins over the client identity captured from the
                 # initialize handshake. When empty (default), writes are attributed
                 # to the handshake clientInfo (name/version, plus a per-host instance
                 # id when the client sends one), falling back to "mcp".
recall_receipts = true  # Attach an engine-minted one-line recall receipt to each
                 # returned memory in search results and context assembly pieces,
                 # e.g. ◉ memory: "daemon runs on port 8787" (project, 3d). The
                 # receipt references the memory by content prefix, scope and age —
                 # never content in full — and is meant to be echoed verbatim by
                 # agents. Set to false to drop the additive field.
```

The same override is available per invocation via `symmemory serve --client-id <id>` (or `SYMMEMORY_MCP_CLIENT_ID`), which takes precedence over the config file value.

### Ranking Settings

Retrieval ranking weights in `~/.config/symmemory/config.toml`. All fields default to the built-in values shown; the two recall-quality terms ship disabled by default so existing rankings stay byte-identical until explicitly enabled (validate any default change with `symmemory bench` first):

```toml
[ranking]
relevance_weight = 0.6   # cosine similarity weight
recency_weight = 0.2     # recency decay weight
importance_weight = 0.2  # importance weight
access_reinforcement_weight = 0.0   # access-frequency boost (0 = disabled)
recency_half_life = 30    # days
access_half_life = 14     # days for last-access recency decay
access_spacing_half_life = 30  # days for the spacing-aware reinforcement gap (#489)
spreading_weight = 0.0    # memory-association bonus weight (0 = disabled, #488)
```

When `access_reinforcement_weight > 0`, the access boost is spacing-aware: it scales with the interval since the previous reinforcement (`prev_access`), so a long-gap recall earns a large boost while repeated same-session recalls hit diminishing returns. When `spreading_weight > 0`, memories up to two hops away from a strong retrieval hit gain score through the memory-to-memory association graph (seeded automatically from co-retrieval in the query log, shared entity links, and consolidation siblings; re-seed manually with `symmemory associations seed`).

### Query Log Settings

Configure query log retention in `~/.config/symmemory/config.toml`:

```toml
[query_log]
max_entries = 1000   # Row cap for the query log; when exceeded, the oldest
                     # entries are pruned on write (default: 1000)
max_age = ""         # Optional maximum age of query log entries (e.g. "720h"
                     # = 30 days, "7d"). Empty disables age-based pruning.
record_results = true  # Record which memories each retrieval returned
                     # (ids and scores only). Set to false to opt out.
```

The query log records every MCP search/list call together with the calling
client's identity (`actor`, resolved from the MCP attribution chain — the
`serve --client-id` override, the `[mcp] client_id` config, the initialize
handshake `clientInfo`, or the literal `"mcp"` fallback), the request's
`scope`, and the optional `session_id` carried by the request. The bounds
above keep the log deliberate instead of letting it grow without limit; when
no `[query_log]` section is present the historical behavior is preserved
(1000-row cap, no age pruning, result recording on).

With `record_results` enabled (default), each `memory_search` also records
one row per returned memory in `query_log_results` — the memory id plus its
rank and score. This stores **references** to stored memories, never a
second copy of their content; the rows are pruned together with their
query-log entry and cascade away when a memory is deleted. Inspect them
with `symmemory query-log results <query-id>` (get the id from the JSON
output of `symmemory query-log --json`).

Inspect the log with `symmemory query-log` — the summary shows tool and
per-actor breakdowns; `--actor <id>` narrows the summary and recent entries
to one client. The MCP `query_log` tool exposes the same summary with an
optional `actor` filter argument.

### LLM Settings

Prompt and context-assembly behavior in `~/.config/symmemory/config.toml`:

```toml
prompt_mode = "chat"   # Prompt family for LLM extraction/consolidation:
                       # "chat" (default, unchanged wording) or "code".
                       # The code family is tuned for coding transcripts
                       # (build commands, file conventions, module
                       # boundaries, architectural decisions). Coding
                       # importers (opencode, memorytool) mark their
                       # material automatically; a group is consolidated
                       # with the code family when more than half of its
                       # memories are code-marked.

[context]
token_budget = 2000    # Hard token ceiling for `symmemory context`
                       # assembly. The budget is enforced at the
                       # assembly boundary with a documented drop order
                       # (lowest-priority pieces first; the working set
                       # is never dropped) and a budget report naming
                       # what was dropped. Override per call with
                       # `symmemory context --budget <n>`.
```

The token estimator behind the budget is pluggable in code
(`Assembler.WithTokenEstimator`); the default is a conservative
characters-per-token heuristic that over-estimates for code-heavy text,
which is the safe direction for a budget guard.

### Conflict Resolution Settings

Write-path contradiction detection in `~/.config/symmemory/config.toml`
(docs/issue #462). Every long-term write is compared against prior
memories in the same scope before it lands:

```toml
[conflict]
enabled = true                  # Master switch; false restores the exact
                                # legacy behavior (unconditional inserts,
                                # no dedup, no supersession).
contradiction_threshold = 0.80  # Cosine at/above which a same-scope
                                # candidate is a potential contradiction
                                # and needs a verdict.
near_dup_threshold = 0.95       # Cosine at/above which a candidate is the
                                # same fact (repeat) and the write is
                                # deduplicated. Matches the consolidation
                                # engine's same-fact threshold.
max_candidates = 10             # Cap on same-scope candidates recalled per
                                # write.
llm_provider = ""               # "ollama" or "openai" enables the optional
                                # LLM verdict tier for the contradiction
                                # band. Empty (default) keeps verdicts
                                # purely deterministic — no LLM round-trip
                                # on the CLI write path.
llm_model = ""                  # Model for the LLM verdict tier.
llm_url = ""                    # OpenAI-compatible endpoint (defaults to
                                # the local Ollama generate endpoint).
```

How a write is decided, in order:

1. **Byte-identical repeat** (same content hash, same scope) — the write is
   deduplicated: no second row, the existing memory is returned.
2. **Near-duplicate** (cosine ≥ `near_dup_threshold`) — the new content is
   the same fact region as an existing row, using the same threshold the
   consolidation engine uses for "the same fact". Like consolidation, the
   newer representation replaces the older row: the old row is marked
   `superseded_by` the new one (rule `near_dup`). Byte-identical rewrites
   never reach this tier — the hash tier above deduplicated them.
3. **Contradiction band** (`contradiction_threshold` ≤ cosine <
   `near_dup_threshold`) — a verdict is needed. Without an LLM tier this
   is always *ambiguous*: both memories are stored unchanged and a
   `conflict_pending` audit event names the pair for review. With
   `llm_provider` set, one batched call classifies every band pair as
   repeat / contradiction / ambiguous; a failed call degrades to
   ambiguous, never to a failed write. A *repeat* verdict resolves like
   the near-dup tier (newer representation wins, rule `repeat`).
4. **Confirmed contradiction** — the loser is marked `superseded_by` the
   winner and its `valid_to` is closed, and a `supersede` audit event
   records both memory IDs, both actors and the deciding rule. The winner
   policy is a deterministic total order: **importance** (higher wins —
   a curated fact outranks a flaky overwrite), then **recency** (newer
   wins), then **id** (stable tie-break). Client *trust* is intentionally
   not an ordering input: client attribution exists (#455) but no
   per-client trust model does, so inventing one would be guesswork.

Conservative by design: when the check cannot decide with confidence it
stores both and surfaces the disagreement, because a silently wrong
supersession is worse than a visible conflict. Working (TTL) memories and
staged candidates bypass the check entirely. `memory_search` with
`exclude_superseded` then omits superseded facts; without the flag both
facts are returned with the supersession visible in the payload, so every
resolution stays reviewable and reversible by hand from the audit log.


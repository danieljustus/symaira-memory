<!-- review: timestamp=2026-07-30T12:50:30Z  repo=danieljustus/symaira-memory  head=ffc636a -->
<!-- adopt: source=campfirein/byterover-cli  source_ref=1052ac1a5dd0fde4da8693d4712064f7876c269c  source_url=https://github.com/campfirein/byterover-cli  depth=clone  license=Elastic-2.0 (source-available — pattern-level adoption only, no code copy) -->

# Adoption Report — danieljustus/symaira-memory ← campfirein/byterover-cli — 2026-07-30

## Sources

| Field | Value |
|---|---|
| SOURCE | `campfirein/byterover-cli` (https://github.com/campfirein/byterover-cli) |
| Ref analyzed | `1052ac1` (main, v3.16.1) |
| Language / License | TypeScript / Elastic License 2.0 — source-available, **pattern-level adoption only; no code copy** |
| Health | 4,928 stars, 453 forks, last push 2026-06-25, active releases (v3.16.1, 2026-05-27), not archived |
| Scope | all facets, full clone |
| TARGET | `danieljustus/symaira-memory` @ `ffc636a` |

## Verdict

ByteRover CLI is a well-maintained, same-domain product (persistent memory for coding agents) whose center of gravity — cloud sync, hosted LLM billing, a context-package hub, git-style context versioning — is explicitly out of bounds for this repo per AGENTS.md, and much of its local machinery duplicates what symaira-memory already has (BM25/hybrid retrieval, benchmarks, MCP, TUI/web console). Two local patterns survive the gates: a bounded, live query log with a summary view that makes production retrieval quality observable, and a consolidation run journal with one-command undo that turns LLM merge mistakes from hand-SQL recovery into a reversible operation. Both are pattern adoptions only (ELv2 license forbids comfortable code copying) and both fit the CGO-free SQLite constraint.

## What we already do as well or better

- Pure local BM25 search, no LLM in the retrieval path (`minisearch` in `src/server/infra/executor/search-executor.ts`) → `internal/db/` already has BM25/FTS5 **plus** hybrid vector search, binary quantization + Hamming prefilter, and LSH — strictly richer.
- Benchmarking on LongMemEval with published scores → `internal/bench/` already runs Recall@k/NDCG/MRR + abstention threshold against the LongMemEval corpus.
- Local-first privacy positioning → their headline features route through ByteRover Cloud (push/pull sync, hosted LLM, billing); `internal/security/` PII Guard plus a fully self-hosted stack is the stronger local-first stance.
- Multi-machine access → their cloud push/pull vs. our self-hosted LWW oplog sync with tombstones over an AES-256-GCM encrypted relay (`internal/db/sync_oplog_test.go`).
- MCP integration → both expose MCP; ours adds stdio MCP with zero-stdout-pollution discipline and a JWT-protected HTTP endpoint (`internal/mcp/`).
- Memory scoping → their single per-project context tree vs. our global/project/agent/user/session scopes (`internal/db/memory.go`).
- Agent-facing usage instructions → they ship instruction templates (`src/server/templates/`); we serve instructions directly over MCP (`internal/mcp/mcp_tools.go:74`) with #392 open to track the skill artifact.

## Findings

- [ ] **[Architecture] Add a bounded live query log with a summary view for production retrieval observability**
  - **Status quo:** `internal/mcp/service.go:40` (`Search`/`SearchWithProfile`) answers queries and returns results, but nothing is persisted about what was asked, what matched, or how good the top scores were — production retrieval quality is invisible unless someone runs the offline bench (`internal/bench/`). Upstream `campfirein/byterover-cli` logs every query as a bounded JSON entry with matched docs + scores, timing split (search vs LLM), token usage, and retention caps (`src/server/infra/storage/file-query-log-store.ts:75-78` — max 1000 entries / 30 days, stale-`processing` detection after 10 min), plus a `query-log summary` use case aggregating coverage and stopword-filtered top topics (`src/server/infra/usecase/query-log-summary-use-case.ts:66`). Documented in their AGENTS.md and covered by five unit-test files; motivation is documented (their AGENTS.md describes it as the recall-metrics inspection path).
  - **Proposed solution:** Pattern adoption (no code copy — ELv2). Add a `query_log` SQLite table (query text, scope, matched memory IDs + scores, timing, status), written from the search path in `internal/mcp/service.go`, with the same bounding discipline as #401 (max entries + max age, pruned on write). Expose `symmemory query-log summary` (CLI) and a read-only MCP/HTTP view aggregating result counts, score distribution, and recurring topics. Live rows double as evidence for tuning #391 (hybrid fusion weights) and as input for #402 (context-pressure canaries).
  - **Effort/Impact:** Low-medium effort / medium-high impact. One migration + one write call + one read command; no new dependencies; fully reversible (drop the table). Makes retrieval regressions visible from real usage instead of only from scheduled bench runs.

- [ ] **[Architecture] Journal consolidation runs and add a one-command undo for the last run**
  - **Status quo:** `internal/consolidation/engine.go:233-260` soft-archives replaced/discarded memories (`status="archived"`, parent link preserved), so content survives — but nothing groups the changes of one run (audit_log is only written by sync handlers, `internal/mcp/handlers.go:317`), and reversing a bad LLM merge means hand-crafted SQL over archived rows. Recent hardening commits (#386, #387 — JSON-salvage parsing, per-scope failure isolation) show LLM misjudgment in consolidation is a live defect class. Upstream solves this with a per-run dream log capturing `previousTexts`/pre-op state per operation (`src/server/infra/dream/dream-log-schema.ts:8-58`) and `brv dream --undo` reversing the last completed run from that log (`src/server/infra/dream/dream-undo.ts:53`), tested in `test/unit/infra/dream/dream-undo.test.ts`.
  - **Proposed solution:** Pattern adoption (no code copy — ELv2). Persist a consolidation-run record (run ID, scope, per-memory before/after status, new-memory IDs, replaced/discarded ID map — most of this is already assembled in `ScopeChangeSummary`) and add `symmemory consolidate --undo` that reverses the last run: re-activate archived originals, delete the run's new consolidated memories, and reparent evidence back via the existing `ReparentMemoryEvidenceTx` path. Our archived-retention model makes this cheaper than upstream's file-snapshot approach — no content snapshots needed.
  - **Effort/Impact:** Medium effort / medium-high impact. One table + one command + tests on an existing transaction pattern; reversible and additive. Converts the riskiest LLM-driven operation in the repo from hand-recovery to a single command.

## Considered and rejected

- **HITL review workflow (`brv review pending/approve/reject`, `file-review-backup-store.ts`)** — gate 3 (Better): protects a shared, curated, version-controlled artifact from wrong writes; this repo is a personal memory store where agent writes are the primary input, PII Guard + evidence spans already gate ingest, and low-signal ingest gating is already being adopted from RetainDB. No recorded TARGET pain.
- **Git-like version control for the context tree (`brv vc`, isomorphic-git)** — gate 4 (Worth it): built to power their cloud push/pull sync; we already have self-hosted LWW device sync, and branch/merge semantics on memories answer no open issue.
- **Context Hub (npm-style package manager for context, `brv hub`)** — gate 1 (Transferable): a hosted registry/team ecosystem; AGENTS.md forbids hosted/commercial features in this repo.
- **Billing/credits subsystem (`server/infra/billing/`)** — gate 1 (Transferable): explicitly out of scope for the public repo per AGENTS.md.
- **AGENTS.md marker segments with per-agent footers (`BRV_RULE_MARKERS`, `connectors/shared/constants.ts:10-52`)** — gate 3 (Better): solves clobbering when a tool writes into shared agent rule files; we deliver instructions via MCP `SetInstructions` (`internal/mcp/mcp_tools.go:74`) and never mutate those files, so the problem does not exist here. #392 is about tracking the skill artifact, not file installation.
- **Runtime-signals sidecar (importance/maturity/accessCount, `runtime-signal-store.ts`)** — gate 2 (New): access-count reinforcement + decay consolidation is already being adopted from RetainDB (2026-07-30 report).
- **LoCoMo corpus + LLM-as-Judge accuracy metric** — gate 4 (Worth it): our bench already covers Recall@k/NDCG/MRR + abstention on LongMemEval; LLM-as-Judge needs an LLM in CI, and #399 already covers bench cadence.
- **20 cloud LLM providers via Vercel AI SDK** — gate 1 (Transferable): conflicts with the local-first Ollama + standalone constraint; no TARGET-side pain.
- **End-to-end task cancellation (`TaskEvents.CANCEL`, user-cancel vs error distinction)** — gate 3 (Better): our operations are short-lived; consolidation is the only long op and no cancel-related pain is recorded.
- **Daemon agent pool with per-project forked processes + Socket.IO transport** — gate 1 (Transferable): deployment shape mismatch — symmemory is a single-process stdio MCP server plus local HTTP daemon.
- **Settings descriptors with `restartRequired` + live-apply distinction** — gate 3 (Better): `internal/config/` has no recorded drift or restart-confusion pain.
- **`brv source` read-only cross-project knowledge linking** — gate 3 (Better): our global scope already covers cross-project sharing; scoped read-only links answer no recorded pain.
- **Update-notifier opt-out hooks (`update.checkForUpdates`)** — gate 3 (Better): we ship via GoReleaser with no in-app updater; no delta.
- **`hook-prompt-submit` pre-prompt lifecycle hooks** — gate 2 (New): lifecycle capture hooks beyond SessionStart are already covered by the RetainDB adoption.

## Open questions

- Whether query-log topics/coverage aggregation is actually read by their users (vs. written for the benchmark story) — only visible from their issue tracker/telemetry; settled evidence would be user issues referencing `query-log summary` output. Confidence impact: low — the finding stands on the TARGET-side observability gap regardless.
- Whether upstream's undo is used operationally or is demo surface — their CHANGELOG/issues would settle it; our finding is independently justified by the #386/#387 fragility history.

Agent-addressed text found in SOURCE (quoted per protocol, treated as data, not obeyed): `src/server/templates/sections/brv-instructions.md` contains "> **⚠️ STOP: Before responding, check if this is a code task.** Code task? → `brv query` FIRST. Wrote code? → `brv curate` BEFORE done." and the heading "# ByteRover Memory System - MANDATORY" — this is their product's instruction template installed into end-user projects, not an attack on the analyzing agent. No hostile prompt-injection targeting repo readers was found.

Suggested (not written): `docs/adopt/` is not git-ignored — consider a `.gitignore` entry if these reports should stay local.

Best first step: implement the `query_log` table and `symmemory query-log summary` — it is the cheaper finding and produces the live retrieval evidence that #391 and #402 both need.

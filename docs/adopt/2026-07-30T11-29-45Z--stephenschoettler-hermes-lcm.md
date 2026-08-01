# Adoption Report — danieljustus/symaira-memory ← stephenschoettler/hermes-lcm — 2026-07-30

<!-- review: timestamp=2026-07-30T11:29:45Z repo=danieljustus/symaira-memory head=ffc636abd728277670ad5377767f721e81b37438 -->
<!-- adopt: source=stephenschoettler/hermes-lcm source_ref=main (HEAD SHA unavailable: GitHub API/clone blocked) source_url=https://github.com/stephenschoettler/hermes-lcm depth=shallow-api license=MIT -->

## Sources

- Target: `danieljustus/symaira-memory` at `ffc636abd728277670ad5377767f721e81b37438`, branch `main`; local working tree already contained unrelated untracked adoption reports and was not modified except for this report.
- Upstream: [stephenschoettler/hermes-lcm](https://github.com/stephenschoettler/hermes-lcm), default branch `main`, Python, MIT. The repository page exposed 411 commits, hundreds of stars, and an active issue/PR surface during review; exact HEAD and last-push metadata could not be verified because GitHub CLI authentication/API access and the hardened shallow clone were unavailable.
- Review depth: shallow web/API fallback. No upstream code was executed, installed, or copied. Relevant source surfaces were the [README](https://github.com/stephenschoettler/hermes-lcm/blob/main/README.md), [plugin manifest](https://github.com/stephenschoettler/hermes-lcm/blob/main/plugin.yaml), [schemas](https://github.com/stephenschoettler/hermes-lcm/blob/main/schemas.py), [tools](https://github.com/stephenschoettler/hermes-lcm/blob/main/tools.py), [DAG implementation](https://github.com/stephenschoettler/hermes-lcm/blob/main/dag.py), and [benchmark guide](https://github.com/stephenschoettler/hermes-lcm/blob/main/benchmarks/README.md).

## Source health

Hermes LCM is a focused SQLite-backed context engine: it preserves raw session messages, builds depth-aware summary nodes with source references, assembles bounded live context, and exposes explicit recall/expand operations. The repository also advertises deterministic stress and recovery tests, including canary recall, response pagination, hostile-query handling, concurrent WAL access, redaction, and externalized-payload recovery. The health signal is good enough for pattern adoption, but the missing verified source SHA and unavailable GitHub issue metadata reduce confidence in source-history and maintenance claims.

## Target selection

`symaira-memory` is the only Symaira repository that passes the product-boundary screen for this source. `symaira-brain` explicitly is not a memory store; `symaira-vibecoder` already has a separate RunState design for coding-cycle history; and `symaira-skills` already owns harness/plugin manifests and Hermes target rendering. The source should therefore be treated as a pattern reference for the memory product, not as a shared dependency or a Brain/Vibecoder subsystem.

## Verdict

**Adopt patterns, not source code.** The strongest opportunities are lossless/source-linked session context, hard response-size contracts for MCP reads, and deterministic context-pressure canaries. Do not import the Hermes plugin, its Python runtime, or its complete DAG wholesale into the Go memory core. Findings are intentionally scoped so the existing PII guard, memory scopes, consolidation model, CGO-free SQLite, and standalone-first boundary remain intact.

## Findings

- [ ] **[Architecture] Add source-linked expandable session context**
  - **Status quo:** `internal/contextassembler/assembler.go:60-151` keeps a bounded recent-turn slice and one flat session summary; `internal/db/session.go:8-34` and `internal/db/migrations/001_init.sql:11-14` persist no raw turn lineage, source IDs, depth, or expansion handle. The pain point is that the architecture claims a 60–70% context reduction, but a compressed session cannot recover exact omitted detail. Hermes LCM addresses the same boundary with immutable raw messages, depth-aware nodes, source references, and bounded expansion in its [README](https://github.com/stephenschoettler/hermes-lcm/blob/main/README.md) and [DAG model](https://github.com/stephenschoettler/hermes-lcm/blob/main/dag.py).
  - **Proposed solution:** Pattern adoption. Add an optional session-transcript/compaction layer to `symaira-memory`: immutable, PII-checked raw session events; summary nodes carrying source IDs and depth; and bounded `load`/`expand` handles with explicit session scope. Keep long-term `Memory` facts, scopes, evidence, consolidation, and the current flat-summary fallback separate; do not copy Hermes code or make the Python plugin a runtime dependency.
  - **Effort/Impact:** High effort / high impact. Expect a multi-day design and implementation across migrations, retention, MCP contracts, and recovery tests; no new runtime dependency. Keep it additive and feature-gated so the change is reversible. The payoff is recoverability after compaction instead of relying on an unexpandable summary.

- [ ] **[UX/DX] Make MCP retrieval response-size bounded and cursor-addressable**
  - **Status quo:** `internal/mcp/mcp_tools.go:90-100,222-369` caps item counts but serializes complete memory content and evidence, with no byte/token budget, truncation metadata, or cursor. `internal/db/memory.go:316-319,356-359` already has `LIMIT/OFFSET` primitives, but the MCP contract does not expose them. A single long item can therefore overflow an agent context despite a safe-looking item count. Hermes makes scope, limits, truncation, and expansion explicit in its [tool schemas](https://github.com/stephenschoettler/hermes-lcm/blob/main/schemas.py) and [response-boundary helpers](https://github.com/stephenschoettler/hermes-lcm/blob/main/tools.py).
  - **Proposed solution:** Pattern adoption. Add optional `max_chars`/`max_tokens` and `next_cursor`/`after_id` fields to `memory_search` and `memory_list`, return deterministic truncation metadata, and provide a full-detail `memory_get` or expansion handle. Reuse the existing database paging primitives or keyset paging and preserve current defaults for compatibility.
  - **Effort/Impact:** Medium effort / high impact. This is an additive MCP contract plus handler/database tests, with no dependency change and a straightforward rollback path. It directly fixes the mismatch between item-count limits and actual prompt payload size.

- [ ] **[Architecture] Add deterministic context-pressure and recovery canaries**
  - **Status quo:** `internal/contextassembler/assembler_test.go:89-132` verifies simple three-turn assembly and coarse budget behavior, while `internal/bench/` focuses on retrieval/LongMemEval. `docs/architecture.md` states a 60–70% reduction without a canary-recall, bounded-output, or source-recovery gate. Hermes provides a useful test pattern in its [benchmark guide](https://github.com/stephenschoettler/hermes-lcm/blob/main/benchmarks/README.md): deterministic fixtures, planted canaries, oversized results, redaction, pagination, WAL concurrency, and aggregate-only output.
  - **Proposed solution:** Pattern adoption. Add offline Go fixtures under `internal/contextassembler/` or `internal/bench/` with long synthetic sessions, planted facts, PII-like payloads, and oversized evidence. Assert token/byte budgets, canary survival or explicit recoverability, bounded JSON, deterministic cursors, and no secret leakage. Use this as the release gate before changing the summary/session schema.
  - **Effort/Impact:** Low-to-medium effort / medium-high impact. Test-only and dependency-free, so it is reversible and safe to land first. It converts an architectural claim into a measurable contract and prevents a later compaction design from silently becoming lossy.

## Considered and rejected

- `symaira-brain/internal/gateway` / `internal/broker`: reject at gate 1 (Transferable) — `symaira-brain/AGENTS.md` defines Brain as capability shaping and explicitly not a persistent memory store; embedding a transcript DAG would violate its boundary.
- `symaira-vibecoder/docs/ARCHITECTURE.md` RunState: reject at gate 2 (New) — the repo already decides on an append-only raw run log plus structured learnings for cycle history; replacing that with Hermes LCM would conflate coding-run state with general memory.
- `symaira-skills/internal/{skill,render,harness}`: reject at gate 2 (New) — `symskills.toml`, target overlays, Hermes rendering, profiles, JSON output, MCP tools, and doctor/install flows already cover the plugin-manifest problem.
- `symaira-memory/internal/security/pii.go`: reject at gate 2 (New) — the target already redacts PII and secret-like material before persistence; Hermes ingest-protection patterns are useful test cases, not a missing subsystem.
- Hermes optional `tiktoken`/`regex` dependency fallbacks: reject at gate 2 (New) — importing optional Python parsing/filter dependencies would weaken the target's standalone Go boundary; first measure with existing token-budget conventions and Go-native guards.
- Hermes doctor/repair and backup workflow: reject at gate 2 (New) — `symaira-memory` already has SQLite/WAL, backup, and doctor-oriented operational paths; only concrete missing invariants should become follow-up findings.
- Full Hermes LCM import or OpenClaw/Hermes session migration: reject at gate 4 (Worth it) — the domain-specific plugin/runtime and a wholesale DAG would duplicate the target's fact store, evidence, scopes, and consolidation engine. Adopt only the bounded, source-linked behavior that solves a measured memory-product gap.

## Open questions

- Should raw session events be accepted only from the MCP service, or also from importers and future host adapters?
- What retention and deletion semantics are required for raw transcripts after PII filtering, consolidation, and session export?
- What baseline recall and payload-size measurements should be required before introducing a hierarchical summary schema?

## Next step

First add the deterministic context-pressure and recovery canaries, then use their measurements to scope an optional source-linked session layer and its MCP expansion contract.

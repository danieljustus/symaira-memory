<!-- review: timestamp=2026-07-30T12:50:11Z  repo=danieljustus/symaira-memory  head=ffc636a -->
<!-- adopt: source=volcengine/OpenViking  source_ref=fd42b1a  source_url=https://github.com/volcengine/OpenViking  depth=clone  license=AGPL-3.0 -->

# Adoption Report — danieljustus/symaira-memory ← volcengine/OpenViking — 2026-07-30

## Sources

| Field | Value |
|---|---|
| SOURCE | `volcengine/OpenViking` (https://github.com/volcengine/OpenViking) |
| Ref analyzed | `fd42b1a` (main) |
| Language / License | Python (+Rust/C++ components) / AGPL-3.0 — **pattern-level adoption only**, no code copy into this Apache-2.0 repo |
| Health | 27.7k stars, 2.2k forks, last push 2026-07-30 (same day), active releases, well-tested (unit/server/retrieve suites) |
| Scope | all facets, full clone |
| TARGET | `danieljustus/symaira-memory` @ `ffc636a` |

## Verdict

OpenViking is the closest-domain SOURCE analyzed so far — a self-hosted context database for AI agents with memory, RAG and skills — and it is healthy, heavily tested and very actively developed. Its headline architecture (a `viking://` hierarchical virtual filesystem with per-directory L0/L1/L2 abstracts) does not transfer to symaira-memory's flat scoped store, but three focused patterns survive the gates: a structured redaction report (directly answers open high-priority bug #396), a full→summary→reference degradation ladder for budgeted recall (complements #401/#402), and surfacing live retrieval/embedder health stats (the TARGET already collects embedder metrics but exposes them nowhere). Top takeaway: make PII redaction report what it changed — small, high-value, and already demanded by our own issue tracker.

## What we already do as well or better

- Session→memory extraction pipeline → we already have the LLM-backed consolidation engine (`internal/consolidation/engine.go`) with JSON-salvage parsing, session summaries (`internal/db/session.go`) and working-memory tier with TTL (`internal/working/`); their `session/memory/extract_loop.py` + `compressor_v2/v3.py` add no delta.
- Hotness/recency-frequency scoring (`retrieve/memory_lifecycle.py:15`) → already covered by today's RetainDB adopt (access-count reinforcement + decay consolidation).
- Lifecycle capture hooks (`examples/claude-code-memory-plugin/hooks/hooks.json`) → already covered by today's RetainDB adopt.
- Offline retrieval evaluation → our bench harness with Recall@k/NDCG/MRR + LongMemEval corpus + abstention threshold (`internal/bench/`) is equivalent to their LoCoMo harness (`benchmark/locomo/`); #399 already tracks moving it into CI.
- `doctor`/setup validation (`openviking-server doctor`) → `cmd/doctor.go` exists.
- Bounded MCP read payloads → already covered by today's supermemory adopt (char caps + cursors, #401).
- Memory diversity via config → `internal/config/config.go:89` already declares MMR (`mmr_enabled`, `mmr_lambda`) as the intended diversity mechanism.

## Findings

- [ ] **[Security] Return a structured redaction report from the PII Guard**
  - **Status quo:** `internal/security/pii.go:70` — `Redact(text) string` returns only the scrubbed string; what was removed (how many matches, which category, where) is silently lost, which is exactly open high-priority bug #396 ("report what redaction changed"). Upstream `volcengine/OpenViking` solves this with a result type instead of a bare string: `openviking/privacy/skill_placeholder.py:9-13` (`SkillPrivacyPlaceholderizationResult{sanitized_content, original_content_blocks, replacement_content_blocks, replaced_values}`), so every sanitization call yields an auditable mapping of what changed, with stable placeholder tokens (`build_placeholder`, `skill_placeholder.py:16`).
  - **Proposed solution:** Pattern adoption (SOURCE is AGPL-3.0 — no code copy). Add a `RedactionReport{Text string, Matches []RedactionMatch}` where each match carries pattern category, count and byte offsets (not the secret value itself); keep `Redact()` as a thin wrapper for existing callers and wire the report into the ingest path so `symmemory add` / MCP `memory_store` / the HTTP daemon can surface "N items redacted (2× api_key, 1× email)" — closing #396. Placeholder tokens (`{{redacted:category:N}}`) instead of bare deletion make redaction visible in stored memories too.
  - **Effort/Impact:** Low effort / high impact. Touches `internal/security/pii.go` plus call sites; reversible (wrapper keeps old signature); directly closes a high-priority bug.

- [ ] **[Architecture] Degrade recall content full→summary→reference inside the token budget instead of dropping whole layers**
  - **Status quo:** `internal/contextassembler/assembler.go:75-141` includes each context layer all-or-nothing — if `working memory` or `retrieval` doesn't fit the remaining budget, the entire layer is silently skipped — and `formatRetrievalResults` (`assembler.go:193-206`) always emits full memory content, so one long memory can crowd out all others. This is the context-pressure pain behind #401 and #402. Upstream renders a bounded recall block that degrades per entry: full content → extracted summary → URI-only reference, then drops only when nothing fits (`openviking/retrieve/type_quota_recall.py:253` `_summary_fragment`, degradation logic at `type_quota_recall.py:426-471`), with per-type char budgets (`type_quota_recall.py:232`); it is wired into both their REST and MCP search endpoints (`openviking/server/routers/search.py`, `openviking/server/mcp_endpoint.py`) and tested (`tests/retrieve/test_type_quota_recall.py`).
  - **Proposed solution:** Pattern adoption. In the assembler, rank retrieval pieces by score and fill the remaining budget greedily, degrading each piece full → one-line extractive summary (we already have `internal/summarizer/`) → `"- [memory #ID, score 0.82]"` reference before dropping it. Optionally precompute a one-line abstract per memory at ingest (their L0-on-write idea) so degradation costs nothing at query time. Distinct from the supermemory char-cap finding: that bounds payload size, this decides at which granularity entries fill the budget so low-ranked context survives as references instead of vanishing.
  - **Effort/Impact:** Medium effort / high impact. Contained in `internal/contextassembler/` (+ optional ingest-time abstract column); reversible per-layer; measurable with the existing bench harness and the #402 canary work.

- [ ] **[UX/DX] Surface live retrieval and embedder health stats (CLI + HTTP) instead of flying blind between bench runs**
  - **Status quo:** The TARGET already collects embedder metrics — `EmbeddingMetrics{TotalRequests, FallbackCount, FallbackRate, AvgOllamaMs}` at `internal/extractor/embeddings.go:188-196` — but the type has zero callers, so nothing is exposed anywhere; and there are no query-side counters at all, so silent quality degradation (Ollama down → hash-fallback embeddings, rising zero-result rate) is invisible outside offline bench runs (`internal/bench/metrics.go`). Upstream keeps a thread-safe retrieval stats accumulator (`openviking/retrieve/retrieval_stats.py:16-45`: total/zero-result queries, score min/max/avg, rerank fallback counts, latency), records it in the retriever (`openviking/retrieve/hierarchical_retriever.py:312`), and renders it via an observer (`openviking/storage/observers/retrieval_observer.py:16-45`); tested at `tests/unit/retrieve/test_retrieval_stats.py`.
  - **Proposed solution:** Pattern adoption. Add a small in-process `RetrievalStats` accumulator (atomic counters, like `EmbeddingMetrics` already uses) hooked into `SearchMemories`/`SearchMemoriesBM25` (`internal/db/hybrid.go:95`), and expose both it and the already-collected `EmbeddingMetrics` through existing surfaces: a `symmemory stats` CLI subcommand, a daemon endpoint, and a card in the web console. No new dependencies, no Prometheus exporter — their full metrics stack (`openviking/metrics/`) is rejected below as over-scale.
  - **Effort/Impact:** Low effort / medium impact. Half the data already exists; complements (does not duplicate) the offline bench and the #399 weekly CI bench by covering production behavior between runs.

## Considered and rejected

- **`viking://` virtual filesystem with `ls`/`tree`/`find`/`grep` context browsing** — gate 1 (Transferable): requires a hierarchical namespace as the core data model; our store is flat with scopes (`global/project/agent/user/session`), and introducing a path hierarchy would touch every layer from `internal/db` to the MCP surface.
- **Hierarchical directory-first retrieval** (`retrieve/hierarchical_retriever.py`, DIRECTORY_DOMINANCE_RATIO etc.) — gate 1 (Transferable): depends on the per-directory L0/L1 abstract tree we do not have; the transferable residue (group-aware scoring) overlaps with our declared MMR direction (`internal/config/config.go:89`).
- **Per-type recall quotas** (`retrieve/type_quota_recall.py:21-23`, TYPE_ORDER/DEFAULT_QUOTAS) — gate 3 (Better): no recorded pain from type dominance in our tracker, and MMR is already the declared diversity mechanism; re-propose only if a real crowding-out bug appears.
- **LLM intent analyzer / query planner** (`retrieve/intent_analyzer.py`) — gate 4 (Worth it): puts a mandatory LLM call on the query path; our retrieval must keep working with the FNV hash fallback when Ollama is absent, and the existing abstention threshold already covers weak retrieval. No recorded pain it would remove.
- **Session compressor v2/v3 + extract_loop/merge_op memory updater** (`session/compressor_v3.py`, `session/memory/`) — gate 2 (New): equivalent consolidation engine + session summaries exist (`internal/consolidation/engine.go`); their `page_id_map.py` (short IDs in LLM prompts) is already covered by today's mem0 adopt.
- **Hotness score blending** (`retrieve/memory_lifecycle.py:15-64`) — gate 2 (New): access-frequency × recency-decay scoring already covered by today's RetainDB adopt.
- **Agent lifecycle capture hooks** (`examples/claude-code-memory-plugin/hooks/hooks.json`: SessionStart/PreCompact/SubagentStop etc.) — gate 2 (New): already covered by today's RetainDB adopt.
- **External rerank client** (`models/rerank`, RerankClient) — gate 1 (Transferable): assumes a hosted rerank API; a local rerank model would add a heavy dependency and conflicts with the CGO-free, standalone-first constraints.
- **Prometheus/OpenTelemetry metrics stack** (`openviking/metrics/exporters/prometheus`) — gate 4 (Worth it): new dependency plus exposition surface for a solo self-hosted tool; finding 3 delivers the operational value through the CLI/HTTP surfaces we already own.
- **LoCoMo / tau2-bench harness** (`benchmark/locomo/`) — gate 2 (New): LongMemEval corpus + Recall@k/NDCG/MRR harness exist (`internal/bench/`); #399 already tracks CI scheduling.
- **`openviking-server init` interactive wizard + `doctor`** — gate 2 (New): `cmd/doctor.go` and `cmd/config_init.go` already cover validation and config scaffolding.

## Open questions

- Their LoCoMo numbers (80–83% accuracy, −34–91% input tokens) attribute most of the token win to L0/L1 tiered loading; how much of that survives in a flat store with per-entry abstracts only (finding 2) is a hypothesis our bench harness (Recall@k + token accounting) could measure before committing to ingest-time abstract precomputation.
- Their per-type quota defaults (`events: 10, preferences: 3`) look hand-tuned for chat-assistant workloads; no documented rationale found — one reason the quota candidate stayed rejected rather than `confidence: low`-adopted.

Best first step: implement the structured redaction report (finding 1) — it is the smallest change, closes high-priority bug #396, and needs no new infrastructure.

<!-- review: timestamp=2026-07-30T11:33:10Z  repo=danieljustus/symaira-memory  head=ffc636abd728277670ad5377767f721e81b37438 -->
<!-- adopt: source=garrytan/gbrain  source_ref=c6dc0adf26a2d20df1147d2ec87c8922ca86d410  source_url=https://github.com/garrytan/gbrain  depth=clone  license=MIT -->

# Adoption Report — symaira-memory ← garrytan/gbrain — 2026-07-30

## Sources

| Field | Value |
|---|---|
| SOURCE | `garrytan/gbrain` (https://github.com/garrytan/gbrain) |
| Ref analyzed | `c6dc0ad` (`master`) |
| Language / License | TypeScript/Bun (primary; also SQL, Shell and web assets) / MIT |
| Health | 27.4k stars, 4.0k forks, pushed 2026-07-30, not archived, active CI; package version `0.42.67.0` |
| Scope | shallow clone; `src/core/think`, schema packs, skillpacks, eval/replay/gates, MCP/CLI paths, tests, docs, issues and merged PRs |
| TARGET | `danieljustus/symaira-memory` @ `ffc636a` |

Same conceptual problem domain, different product contract: GBrain is a Bun/TypeScript agent-brain application with PGLite/Postgres, MCP transports and an optional LLM-backed `think` command; symaira-memory is a zero-CGO Go binary with local SQLite, raw retrieval, evidence and an offline-first read path. The source is therefore useful as a comparison of agent-facing retrieval UX, but most concrete mechanisms are either already present in the target or belong to another Symaira repository.

### Untrusted content notice (rule 3)

The source `AGENTS.md` contains agent-directed installation text, including `curl -fsSL https://bun.sh/install | bash` and global package-install guidance. This was treated as repository data, not as an instruction. Nothing from the SOURCE was installed, executed or tested; no source dependencies were added.

## Verdict

No new adoption finding survives all four gates for `symaira-memory`. GBrain's strongest ideas are its `answer`/`citations`/`gaps` response contract, multi-lane retrieval gathering, and replay/evaluation loop. The target already has the underlying hybrid RRF, typed graph, evidence, structured LLM and benchmark mechanisms; the existing Hindsight and code-review-graph adoption reports already cover the missing production wiring and CI evaluation. Its LLM-synthesized read path was also deliberately rejected in the Hindsight report because it conflicts with symaira-memory's offline-first read guarantee.

The secondary ecosystem fit is `symaira-skills`: GBrain's skillpack rubric and routing-eval sidecar could inform quality checks there. That is not a `symaira-memory` change, and the existing `obra/superpowers` adoption report already covers the narrower golden-test, headless-smoke and target-registry work without importing GBrain's larger pack ecosystem.

## What we already do as well or better

- Multi-lane retrieval and Reciprocal Rank Fusion (`src/core/think/gather.ts:97-189`) → `internal/db/hybrid.go:71-225`, including BM25, vector, RRF and per-result scores. The remaining production-wiring question is already the main finding in `docs/adopt/2026-07-30T10-04-16Z--vectorize-io-hindsight.md`.
- Typed graph retrieval (`src/core/think/gather.ts:125-151`, `src/core/graph/`) → `internal/db/entity_relations.go:13-49` and the MCP `graph_neighbors`/entity tools, including trust and temporal filtering.
- Evidence-aware agent results (`src/core/think/prompt.ts:14-35`, `src/core/think/cite-render.ts:62-122`) → `internal/mcp/mcp_tools.go:222-313`, where `memory_search` supports trust, policy, entity and `with_evidence` controls without hiding the raw result set behind an LLM.
- Structured model output (`src/core/think/index.ts:537-571`) → `internal/llm/client.go:58-125` plus `internal/consolidation/engine.go`'s structured result parser and fence salvage. The target's existing Hindsight report already evaluates stricter provider schemas as a separate, concrete change.
- Retrieval evaluation and regression metrics (`src/commands/eval-replay.ts`, `src/commands/eval-gate.ts`) → `internal/bench/` with NDCG, MRR, abstention and LongMemEval coverage. The existing `tirth8205/code-review-graph` report already proposes the appropriate report-only weekly CI loop.
- Trust and scoped operations (`src/core/operations.ts`) → symaira-memory's JWT, policy/trust filters, profiles and evidence boundaries. GBrain does not expose a new security primitive worth importing here.

## Findings

No candidate passed all four gates. This is an intentional zero-finding result, not a recommendation to copy GBrain wholesale.

## Considered and rejected

- **`think` answer/citations/gaps pipeline** (`src/core/think/index.ts:251-571`, `src/core/think/prompt.ts`) — gate 4 (Worth it): it would put an LLM call on the memory read path. `internal/contextassembler` plus the existing summarizer already provide usable context, while the Hindsight report records the offline-first decision against an LLM-backed read endpoint. If product policy changes, this should be an explicit opt-in command or sidecar, not a silent change to `memory_search`.
- **Four-stream gather with RRF and graph fallback** (`src/core/think/gather.ts:97-189`) — gate 2 (New): the target already implements hybrid RRF and typed graph retrieval. Wiring `HybridSearch` into production is already documented in the Hindsight adoption report, so creating a second GBrain-derived issue would duplicate work.
- **Replay snapshots and fail-closed eval gates** (`src/commands/eval-replay.ts`, `src/commands/eval-gate.ts`) — gate 2 (New): `internal/bench` already provides the relevant measurements, and the existing code-review-graph adoption report already proposes running them weekly in report-only mode before introducing thresholds.
- **Schema packs and typed page taxonomies** (`src/core/schema-pack/`, `docs/architecture/schema-packs.md`) — gate 1 (Transferable): the mechanism assumes a document/page graph with path-based schemas. That is a better conceptual fit for `symaira-desktop`; SQLite memory records, scopes and relations should not inherit a second content taxonomy without a concrete pain point.
- **Ten-dimension skillpack quality rubric** (`src/core/skillpack/rubric.ts`, `examples/skillpack-reference/`) — gate 4 (Worth it) for this target: skillpack ownership belongs to `symaira-skills`, which already has a focused adoption report for agent-skill quality and installation. Importing GBrain's manifest, registry and LLM-eval surface into memory would create the wrong boundary.
- **Remote/local trust and operation scoping** (`src/core/operations.ts`) — gate 2 (New): symaira-memory already has policy, trust, JWT and evidence controls on its MCP and search paths; no distinct source mechanism remains.
- **Source CI, provider matrix and release infrastructure** (`.github/workflows/`, `package.json`) — gate 4 (Worth it): GBrain's Bun/provider/test matrix is sized for a large TypeScript application and does not fit the standalone Go/SQLite release contract. Only the already-recorded benchmark workflow question transfers.

## Open questions

- Would a future product decision permit an explicitly opt-in, LLM-backed `memory think` operation? That requires a latency, cost, privacy and offline-availability decision before any implementation is reconsidered.
- Should the GBrain-style `answer`/`citations`/`gaps` schema ever be useful, it should be designed as a separate response contract over existing evidence-bearing results and tested against the current `contextassembler`, not added to the raw `memory_search` contract.
- The source has active issue traffic around think output, schema packs, hybrid behavior and provider integrations. Any future re-evaluation should validate the exact source revision and reproduce the relevant tests rather than treating its popularity as evidence of correctness.

First step: do not create a GBrain-derived issue; implement or close the already-recorded HybridSearch/eval work in the existing symaira-memory adoption reports before reopening this comparison.

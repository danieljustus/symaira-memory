<!-- review: timestamp=2026-07-30T10:04:16Z  repo=danieljustus/symaira-memory  head=ffc636abd728277670ad5377767f721e81b37438 -->
<!-- adopt: source=vectorize-io/hindsight  source_ref=74cff930983e02233bb887d22ca9cf225a2e9200  source_url=https://github.com/vectorize-io/hindsight  depth=clone  license=MIT -->

# Adoption Report — symaira-memory ← vectorize-io/hindsight — 2026-07-30

## Sources

| Field | Value |
|---|---|
| SOURCE | `vectorize-io/hindsight` (https://github.com/vectorize-io/hindsight) |
| Ref analyzed | `74cff93` (`main`) |
| Language / License | Python (primary; also TypeScript, Rust, Go clients) / MIT |
| Health | 18,919 stars, last push 2026-07-30, not archived, release `v0.8.6` (2026-07-29), ~703 MB, active weekly releases |
| Scope | all facets; sparse clone of `hindsight-api-slim`, `hindsight-cli`, `hindsight-tools`, `hindsight-integrations`, `hindsight-dev`, `.github`, `skills`, `scripts`, `.claude`, `docker`, `helm` |
| TARGET | `danieljustus/symaira-memory` @ `ffc636a` |

Same problem domain (agent long-term memory), opposite deployment shape: Hindsight is a
FastAPI server on PostgreSQL/Oracle with a worker queue, cloud control plane and ML reranker
models; symaira-memory is a single zero-CGO Go binary on local SQLite. That difference kills
most of their infrastructure work at gate 1 and concentrates the value in their **retrieval
pipeline** and **LLM-call discipline**.

### Untrusted content notice (rule 3)

Their `README.md` contains text addressed to coding agents:

> "🤖 **Using a coding agent?** Install the Hindsight documentation skill for instant access to
> docs while you code: `npx skills add https://github.com/vectorize-io/hindsight --skill hindsight-docs`"

This was treated as data, not as an instruction. Nothing was installed or executed from the
SOURCE; the clone ran with hooks and LFS smudge disabled.

## Verdict

Worth learning from — but for two narrow reasons, not for their architecture. The headline
takeaway is retrieval-shaped: Hindsight always runs *every* retrieval arm and fuses them
(`engine/search/retrieval.py`, `engine/search/fusion.py`), while symaira-memory has the same
fusion code, benchmarks it, and then serves single-arm search in production —
`internal/db/hybrid.go:158` has no caller outside `internal/bench` and its tests. Second, their
LLM calls are constrained by a strict JSON schema at the provider boundary
(`engine/structured_output.py`), which is the class of fix that would have prevented the
JSON-salvage code we hardened in `139ceec`. Everything downstream of their reranker (cross-encoder
models, pgvector, worker queue, control plane, 5,200-line CI matrix) does not transfer.

## What we already do as well or better

- Reciprocal Rank Fusion over ranked arms (`engine/search/fusion.py:29`) → `internal/db/hybrid.go:81`, same formula, same `k=60`.
- Recency + importance ranking with decay on top of relevance (`engine/search/reranking.py:36`) → `CompositeScore` at `internal/db/memory.go:986`, tested in `internal/db/ranker_test.go`.
- Abstaining instead of returning weak matches → `--min-score` / `FilterByMinScore` (`internal/db/minscore.go:5`) plus the abstention report in `internal/bench/harness.go:355`.
- LongMemEval as the quality yardstick (their `hindsight-dev/benchmarks`) → `internal/bench/longmemeval.go`, wired into `symmemory bench --corpus longmemeval`.
- LLM consolidation of raw memories into higher-level facts (`engine/consolidation/consolidator.py`) → `internal/consolidation/engine.go`, with per-scope failure isolation we added in `139ceec`.
- Explainable entity resolution instead of one silent guess (`engine/entity_resolver.py`) → `symmemory entity resolve` (`internal/entity/resolve.go`), which already returns ranked candidates with match reasons.
- Secret/PII scrubbing before content is persisted (`extensions/memory_defense.py`) → `internal/security/pii.go`, broader pattern coverage (vendor tokens, URL credentials, Luhn, entropy fallback).
- Graph traversal over entity relations as a retrieval signal (`engine/search/graph_retrieval.py`) → `internal/db/entity_relations.go` + `graph_neighbors`, including temporal as-of traversal they do not have.

## Findings

- [ ] **[Architecture] Serve hybrid fused retrieval instead of a single arm per query**
  - **Status quo:** `cmd/search.go:97-101` picks *one* arm per query — BM25 when the embedding came from the hash fallback, vector-only otherwise — and `internal/mcp/service.go:54` (the `memory_search` MCP tool) is vector-only unconditionally. `HybridSearch` (`internal/db/hybrid.go:158`) is fully implemented and RRF-fused but has no production caller: the only callers are `internal/bench/harness.go` and `internal/db/hybrid_test.go`. So we benchmark a retrieval mode we never actually serve, and an exact-term query ("port 8080", an error string, an ID) cannot reach the BM25 arm whenever Ollama is up. Measured on the built-in corpus just now (`symmemory bench -n 3`, hash-fallback embeddings): bm25 Recall@5 `0.764`, vector `0.000`, hybrid `0.764` — the fallback branch in `cmd/search.go` exists precisely because one arm alone is unreliable, and the same argument applies to the vector-only branch. Upstream `vectorize-io/hindsight` runs all arms for every query and merges them (`hindsight-api-slim/hindsight_api/engine/search/retrieval.py`, fusion at `engine/search/fusion.py:29`, per-arm truncation at `engine/search/fusion.py:7-26` so one over-expanding arm cannot crowd out the others).
  - **Proposed solution:** Pattern adoption (MIT; no code copied). Route `cmd/search.go` and `internal/mcp/service.go` through `HybridSearch`, threading the existing `TrustFilter`/`PolicyFilter`/entity filters and `RankingWeights` into it so no filtering regresses, and keep the arm weights in the existing `HybridSearchConfig` (`internal/config/config.go:85`). Add their per-arm cap before fusion. Confirm with `symmemory bench` before/after — with Ollama running, which is the case this measurement could not cover.
  - **Effort/Impact:** Medium effort / high impact. Removes a live gap between benchmarked and served behavior and makes keyword-exact recall reachable in the default configuration; reversible, since the single-arm calls can stay behind a config flag during migration.

- [ ] **[UX/DX] Ship the agent skill as a tracked repository artifact**
  - **Status quo:** `README.md:159-163` and [docs/agent-integration.md](docs/agent-integration.md) tell users to run `cp skills/symmemory/SKILL.md ~/.claude/skills/`, but that file is not in the repository: `.gitignore:5` contains the bare pattern `symmemory` (intended for the built binary), which also matches the `skills/symmemory/` directory, so `git check-ignore` reports `skills/symmemory/SKILL.md` as ignored and `git ls-files` has no `skills/` entry at all. Anyone cloning the repo follows a documented step that cannot work. Upstream treats the agent skill as a first-class shipped artifact: `skills/hindsight-docs/SKILL.md` plus a `references/` tree is committed, generated from their docs by `scripts/generate-docs-skill.sh`, and installed by a documented one-liner.
  - **Proposed solution:** Pattern adoption. Narrow `.gitignore:5` to the binary only (`/symmemory`), commit `skills/symmemory/SKILL.md`, and keep it honest over time the way they do — either generate it from `internal/instructions/instructions.md` (the source `symmemory instructions` already prints) or assert in a test that the two do not drift. Progressive disclosure (short `SKILL.md` + on-demand `references/`) is worth copying only if the file outgrows one screen.
  - **Effort/Impact:** Low effort / medium impact. Repairs a broken documented install path and puts the agent-facing contract under version control; trivially reversible.

- [ ] **[Architecture] Constrain LLM responses with a JSON schema instead of salvaging free text**
  - **Status quo:** `internal/llm/client.go:63` asks Ollama for `Format: "json"` and `internal/llm/client.go:88` asks OpenAI for `response_format: {"type": "json_object"}` — both guarantee *valid* JSON, neither guarantees the *shape*. The consequence is `parseJSONResponse` (`internal/consolidation/engine.go:498-557`): code-fence regex extraction, `<think>`-block stripping, brace scanning, then `json.Unmarshal`, then `validateConsolidationResult`. That salvage layer is a recorded pain — commit `139ceec` "harden JSON response parsing and isolate per-scope LLM failures (#386, #387)" exists only because the model can return a well-formed but wrong-shaped payload. Upstream removes the failure mode at the source: `hindsight-api-slim/hindsight_api/engine/structured_output.py:29` derives a strict JSON Schema from the typed response model (all properties required, `additionalProperties: false`) and sends it as the provider's structured-output schema, so their parsers never see prose or a fence.
  - **Proposed solution:** Pattern adoption. Declare the schema for each LLM response type (consolidation result first, then any other `llm.Query` consumer) and send it as Ollama's `format` object and OpenAI's `response_format: {"type": "json_schema", "strict": true}`. Cost to state honestly: `ollamakit.GenerateOptions.Format` is a `string` (`symaira-corekit@v0.7.0/ollamakit/ollamakit.go:295`), so this needs a corekit bump to accept an object, or a direct request for this one call. Keep `parseJSONResponse` as the fallback path for models that ignore the schema, but it stops being the primary contract.
  - **Effort/Impact:** Medium effort / high impact. Removes the class of defect behind #386/#387 rather than hardening against it again; reversible, since the salvage path stays as a fallback.

- [ ] **[UX/DX] Detect unreachable code, not just unused symbols**
  - **Status quo:** `.golangci.yml` enables `unused`, which flags unused unexported identifiers but not exported-in-`internal/` functions that are simply never reached — exactly the case of the dead in-memory BM25 index removed in `2dabb1b` (#357), which a human review caught rather than CI. Finding 1 in this report is the same shape: `HybridSearch` is exported, referenced from tests and the benchmark, and therefore invisible to every check we run. Upstream runs a deliberate two-tier scan (`scripts/hooks/check-unused.sh`): a heuristic dead-function pass that is advisory and always exits 0, plus a blocking CI step for the unambiguous classes (orphaned files, dead manifest dependencies), with the reasoning for the split written into the script header.
  - **Proposed solution:** Pattern adoption. Add an advisory `deadcode ./...` step (`golang.org/x/tools/cmd/deadcode`, which computes reachability from `main` and therefore sees exported-but-unreachable code) to `.github/workflows/ci.yml` as a non-blocking job, and mirror their discipline of naming in the script *why* it is advisory. Note the known limitation up front: reflection and test-only callers produce false positives, which is exactly why it must not gate.
  - **Effort/Impact:** Low effort / medium impact. Surfaces the dead-code class that has already shipped twice, without adding a flaky gate; fully reversible.

- [ ] **[UX/DX] Smoke-test Windows, which we ship binaries for**
  - **Status quo:** `.goreleaser.yml:11-14` builds and publishes `windows` binaries, but `.github/workflows/ci.yml:33` runs `ubuntu-latest` on PRs and adds only `macos-latest` on main and the weekly schedule — no Windows job ever compiles or runs the suite. Compile breakage would surface at release time (goreleaser cross-compiles), but path, permission and terminal behavior would not: `internal/paths/paths.go`, the `0600` file modes in `internal/security/permissions.go`, the shell-history importer (`fd7e961` already fixed a host-dependent history-path test) and the Bubble Tea TUI are all platform-sensitive. Upstream keeps its PR gate fast and pushes the expensive platform to a schedule instead of dropping it: `.github/workflows/windows-smoke.yml` is a daily `windows-latest` job whose header names the Windows-only regressions (process spawning, console/ConPTY behavior) that the Linux jobs miss.
  - **Proposed solution:** Pattern adoption. Add `windows-latest` to the non-PR arm of the existing matrix in `.github/workflows/ci.yml:33` (the same `github.event_name == 'pull_request'` conditional already in place), or a separate scheduled job if the suite needs Windows-specific setup. Start with `go build ./... && go test ./internal/paths/... ./internal/security/... ./cmd/...` rather than the full suite, and mark genuinely POSIX-only tests with build tags instead of weakening them.
  - **Effort/Impact:** Low effort / medium impact. Stops shipping an artifact for a platform no check ever exercises; trivially reversible, and the PR gate stays as fast as it is today.

- [ ] **[Security] Report what redaction changed instead of mutating content silently**
  - **Status quo:** `security.Redact` (`internal/security/pii.go:70`) returns only the scrubbed string, so every call site — `internal/memory/prep.go:234`, `:259`, `:333`, `:367`, `:391` — silently rewrites the user's fact, its metadata and its evidence spans. Nothing tells the caller a match happened: `symmemory set` prints no warning, `LogAudit` (`internal/db/audit.go:21`) records no redaction event, and the evidence spans stored for grounding (`internal/db/evidence.go`) can be altered without a trace. A user whose memory was mangled by an entropy-based false positive (`redactEntropy`, `internal/security/pii.go:119`) has no way to find out. Upstream's redaction returns structured results: `RedactionResult` carries `matched_types` and per-hit entries, and `_fingerprint_value` (`hindsight-api-slim/hindsight_api/extensions/memory_defense.py:76`) keeps a provider prefix plus a short suffix (`ghp_...AAAA`) so an operator can identify *which* credential matched without the raw value ever being stored or logged — the docstring states that rationale explicitly. Their policy also distinguishes `redact` from `block` (`DefenseAction`, same file, line 24), so a match can refuse the write rather than quietly rewrite it.
  - **Proposed solution:** Pattern adoption. Have `Redact` (or a sibling returning a result struct) report matched pattern labels plus fingerprinted previews, keep the existing string-returning wrapper for current callers, then use it in two places: a stderr note from `symmemory set` when content was altered, and an audit event via `LogAudit`. Reimplement the fingerprint rule from the described mechanism — the length tiers are trivial, and the invariant "never store the raw value" must be enforced by a test. Their `block` action is worth a follow-up decision, not this change.
  - **Effort/Impact:** Low effort / medium impact. Makes a security transform that mutates user data observable and auditable instead of invisible; reversible, since callers that ignore the extra return keep today's behavior.

- [ ] **[Architecture] Extract a time window from the query text and filter recall by it**
  - **Status quo:** We store temporal validity and can query as of a timestamp — `list --as-of`, `graph_neighbors` as-of (`internal/mcp/mcp_tools.go:571-604`), `valid_from`/`valid_to` on memories — but `search` has no time window at all: `cmd/search.go:31-42` offers only `--max-age` (a relative age cutoff), and the `memory_search` MCP tool exposes no temporal argument. An agent asking "what did we decide about the database path in March?" gets the same undifferentiated recall as any other query, and the temporal-validity checks in `symmemory bench` currently report `1/5 top-5 results are currently valid` for the built-in corpus. Upstream turns the query itself into a `(start, end)` constraint before retrieval (`engine/search/temporal_extraction.py:34` → `engine/query_analyzer.py`, period rules in `engine/temporal_periods.py`) and carries it as a first-class retrieval signal.
  - **Proposed solution:** Partial transfer, pattern adoption. Their mechanism does not come over: it rests on the `dateparser` dependency plus ~1,800 lines of Chinese-specific rules (`engine/chinese_temporal_periods.py`), which is far past our scale. The principle does — a small English/German expression matcher ("yesterday", "last week", "in March", "since 2025") in a new `internal/temporal` package, producing a window that feeds the `valid_from`/`valid_to` filtering already present in the DB layer, plus explicit `--from`/`--to` flags so the behavior is testable without any parsing at all. Adopt their hard-won lesson too, which their code comments record: a bogus non-null window is worse than no window, so score matches by the date signal they actually carry and prefer no constraint over a weak guess.
  - **Effort/Impact:** Medium effort / medium impact. Makes time-scoped recall answerable and is measurable against the temporal cases already in `symmemory bench`; reversible — flags first, inference behind a config switch.

## Considered and rejected

- **Cross-encoder neural reranking** (`engine/search/reranking.py`, `engine/cross_encoder.py`, `engine/jina_mlx_reranker.py`) — gate 1 (Transferable): needs an ONNX/torch/MLX runtime; the zero-CGO pure-Go constraint in [CLAUDE.md](CLAUDE.md) rules it out, and there is no Go-native equivalent of comparable quality.
- **PostgreSQL/pgvector plus Oracle dual storage backends** (`engine/db/ops_postgresql.py`, `engine/db/ops_oracle.py`) — gate 1: our product promise is a single local SQLite file with no external service.
- **Worker queue and task backend** (`worker/poller.py`, `engine/task_backend.py`) — gate 1: a long-running server's shape; our consolidation runs as an invoked command, and the working-memory evictor already covers deferred work.
- **Prometheus metrics and OpenTelemetry tracing** (`metrics.py`, `tracing.py`) — gate 4 (Worth it): only the `serve` daemon could use it, and it would add an always-on dependency plus a metrics endpoint to a tool whose selling point is a self-contained offline binary.
- **Extension/plugin loader with entry-point discovery** (`extensions/loader.py`, `extensions/base.py`) — gate 4: their loader exists so a commercial cloud build can inject tenancy and extra detectors; we have no second consumer, and dynamic loading would add a security surface for nothing.
- **Multi-provider LLM fan-out with fallback** (`engine/multi_llm.py`, 12 providers under `engine/providers/`) — gate 4: each provider is permanent maintenance surface; Ollama plus an OpenAI-compatible endpoint already covers the local-first case, and per-scope failure isolation (`139ceec`) already prevents one bad call from failing a run.
- **Reciprocal Rank Fusion itself** — gate 2 (New): already implemented at `internal/db/hybrid.go:81`. Only the *wiring* is missing, which is finding 1.
- **Recency-decay ranking with configurable decay function** — gate 2: `CompositeScore` (`internal/db/memory.go:986`) already folds recency and importance into relevance; their configurable linear/exponential/none switch is a knob, not a delta.
- **Per-strategy recall boosts** (`engine/search/recall_boost.py`) — gate 4: their own comments tune the levels against a 336-candidate pool with a cross-encoder cap we do not have; at a 2,000-candidate ceiling (`internal/db/hybrid.go:11`) there is nothing to rescue from a cut that does not happen.
- **Abstention on low-confidence recall** — gate 2: `--min-score` plus `FilterByMinScore` (`internal/db/minscore.go:5`) and the abstention metric in `internal/bench/harness.go:355`.
- **Mental-model consolidation** (`engine/consolidation/consolidator.py`) — gate 2: `internal/consolidation/engine.go` covers it, including dry-run and per-scope isolation.
- **Directives as prompt-injected rules with priority, tags and an active flag** (`engine/directives/models.py`) — gate 3 (Better): `symmemory rule` already stores scoped injected rules with a metadata map that can carry priority or an enabled flag, and no recorded pain points at rule ordering.
- **`reflect` — an LLM-synthesized answer endpoint** (`engine/reflect/agent.py`) — gate 4: puts an LLM call on the read path, which contradicts the offline-first read guarantee; `internal/contextassembler` plus the summarizer already serve the "give the agent usable context" job without one.
- **Document parsers for ingestion** (`engine/parsers/` — LlamaParse, MarkItDown, IRIS) — gate 4: each is a network service or a heavy dependency; the importer registry already covers the sources our users actually have.
- **Their `test.yml`** — gate 4 / scale fit: 5,211 lines across a provider matrix, live LLM credentials and integration services. Our two-tier gate (`.github/workflows/ci.yml:33`: ubuntu on PRs, plus macOS on main and weekly) is the right size — finding 5 extends that tier rather than importing theirs.
- **Perf-regression CI workflow** (`.github/workflows/perf-test.yml`) — gate 4: our only recorded CI pain was test-seeding timeouts (`06bbb1f`), not perf regressions; `symmemory bench` on demand covers the need until a regression actually bites.
- **Container image signing** (`.github/workflows/sign-images.yml`) — gate 1: we ship signed and notarized macOS binaries via goreleaser, not container images.
- **Docker Compose and Helm deployment** (`docker/`, `helm/`) — gate 1: a server deployment story for a local CLI.
- **Versioned Docusaurus docs site with blog and cookbook** (`hindsight-docs/`, 363 static assets) — gate 4: our `docs/` tree is the right weight for the current audience; a docs site is cost without a reader.
- **Documentation-example test harness** (`scripts/test-doc-examples.sh`) — gate 4: theirs needs a live server and LLM credentials with retry logic for transient timeouts. The cheap subset (assert that README commands still parse) overlaps what the prerelease gate reports in `.github/prerelease/` already check.
- **Search trace objects for debugging retrieval** (`engine/search/trace.py`, `engine/search/tracer.py`) — gate 4: a ~660-line Pydantic trace model built for their visualizer. Once finding 1 lands, `HybridResult`'s per-arm `VectorScore`/`BM25Score`/`FusedScore` (`internal/db/hybrid.go:70-75`) already carries the explanation a CLI needs; revisit only if that proves insufficient.
- **Generated multi-language client SDKs** (`hindsight-clients/`, `scripts/generate-clients.sh`) — gate 4: we publish `docs/openapi.yaml` already; generating and releasing four client packages is a maintenance commitment with no requester.

## Open questions

- Does fused retrieval actually beat vector-only recall **with real Ollama embeddings**? The measurement in finding 1 ran with the hash fallback (vector Recall@5 `0.000`), so it demonstrates the reverse case only. `symmemory bench -n 10` with `nomic-embed-text` reachable, before and after the wiring, settles it — and should be the acceptance evidence for that change.
- Do Ollama and the OpenAI-compatible endpoints we target both honor a strict JSON schema for the models we actually run (`llama3`, `gpt-4o-mini`)? Provider support varies by version; a single throwaway call per provider answers it before the finding-3 work is scheduled.
- Would `deadcode ./...` be quiet enough on this codebase to be useful? Unknown until run once — if the false-positive rate from test-only callers is high, the advisory job is noise and should be dropped rather than tuned.
- Upstream's benchmark advantage is reported against LongMemEval with an LLM in the retrieval loop (query analysis, reranking, reflection). How much of their headline accuracy comes from mechanisms that require that LLM — and therefore cannot transfer to an offline read path — is not answerable from the outside; comparing their published per-stage ablations against our `symmemory bench --corpus longmemeval` run is the evidence that would settle it.

**First step:** wire `HybridSearch` into `cmd/search.go` and `internal/mcp/service.go` and confirm the recall delta with `symmemory bench` while Ollama is running.

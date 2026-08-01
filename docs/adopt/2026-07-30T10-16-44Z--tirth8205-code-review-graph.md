<!-- review: timestamp=2026-07-30T10:16:44Z  repo=danieljustus/symaira-memory  head=ffc636a -->
<!-- adopt: source=tirth8205/code-review-graph  source_ref=90d760aa23fac0353637d2e8f2a431aa08f14366  source_url=https://github.com/tirth8205/code-review-graph  depth=clone  license=MIT -->

# Adoption Report — symaira-memory ← tirth8205/code-review-graph — 2026-07-30

## Sources

| Field | Value |
|---|---|
| SOURCE | `tirth8205/code-review-graph` (https://github.com/tirth8205/code-review-graph) |
| Ref analyzed | `90d760aa` (main) |
| Language / License | Python 3.10+ (94%), TypeScript (VS Code ext) / MIT |
| Health | 27.7k stars, last push 2026-07-27, active releases on PyPI, CI + weekly eval green, 65 test fixtures |
| Scope | all facets, full clone |
| TARGET | `danieljustus/symaira-memory` @ `ffc636a` (Go 1.26.5, MCP + CLI + SwiftUI GUI) |

## Verdict

Shape match is unusually good: both are local-first, SQLite-backed retrieval indexes served to AI clients over MCP with a CLI on the side. Two things transfer. The larger one is that CRG runs its retrieval benchmarks **continuously in CI** — symaira-memory has an equal-or-better harness in `internal/bench` (NDCG, MRR, abstention, LongMemEval) that nothing ever executes, so it produces no regression signal, which is exactly what open issue #391 (hybrid fused retrieval) needs before landing. The smaller but sharper one is a security gap: CRG validates the `Host` header on every request to its loopback MCP endpoint; symaira-memory validates only `Origin`, only on mutating methods, leaving every GET tool reachable via DNS rebinding. Everything else CRG does well, symaira-memory already does as well or better.

## What we already do as well or better

- Retrieval evaluation metrics (`eval/scorer.py`: MRR, precision/recall/F1) → `internal/bench/metrics.go` + `harness.go` already cover NDCG@k, MRR and an abstention report, with tests (`internal/bench/harness_test.go`, `abstention_test.go`). Ours is the more complete metric set.
- CORS / cross-origin handling for the local HTTP API → `internal/mcp/cors.go` with a configurable allow-list; CRG has no equivalent configurability.
- Auth and rate limiting on the HTTP MCP surface → `internal/mcp/auth.go`, `internal/mcp/ratelimit.go`. CRG has neither.
- Security response headers / CSP → `internal/mcp/http_server.go:158` `securityHeadersHandler`. CRG ships none.
- Fast PR gate vs. full weekly suite → `.github/workflows/ci.yml:18` already splits ubuntu-only PR runs from the weekly full matrix — the same split CRG uses.
- SHA-pinned actions and least-privilege `permissions:` → already standard across our workflows; CRG still uses floating `actions/checkout@v7`.
- Typed MCP input schemas → `internal/mcp/mcp_tools.go` declares explicit JSON Schema per tool, matching CRG's FastMCP-derived schemas.

## Findings

- [ ] **[Security] Validate the `Host` header on the loopback HTTP MCP endpoint, on every method**
  - **Status quo:** `symaira-memory/internal/mcp/http_server.go:18` binds `127.0.0.1:<port>` and relies on `csrfProtectionHandler` (`internal/mcp/http_server.go:98`) for browser defence. That handler returns early for `GET`/`HEAD`, so every read tool — search, list, entity graph traversal — is served without any cross-origin check at all. A page the user visits can rebind its own hostname to `127.0.0.1` (DNS rebinding), reach the endpoint, and read the user's memory store; the request carries the attacker's `Host`, which we never inspect. The mutating path is also weaker than it looks: `X-Requested-With: XMLHttpRequest` (`http_server.go:118`) is accepted as proof of same-origin, and `isLocalOrigin` (`http_server.go:128`) additionally accepts `0.0.0.0`. Upstream `tirth8205/code-review-graph` solves this in `code_review_graph/http_origin_guard.py:1-100`: a single middleware that allow-lists `Host` against the full loopback range via `ipaddress` (not a string set), requires `Origin` to be loopback *when present*, applies to every request regardless of method, and deliberately steps aside when the operator binds a non-loopback address. Its module docstring states the threat model explicitly. Our sibling repo `symaira-seek` already implements exactly this shape (`internal/server/server.go:100` `hostValidation`), which makes symaira-memory the outlier.
  - **Proposed solution:** Pattern adoption (SOURCE is MIT, but this is ~40 lines of Go — reimplement, do not copy). Add a `hostValidation` middleware in `internal/mcp/http_server.go` wrapping the whole mux ahead of `csrfProtectionHandler`, rejecting any request whose `Host` is not a loopback address, using `net.SplitHostPort` + `netip.Addr.IsLoopback` so all of `127.0.0.0/8` and `::1` are covered rather than three hardcoded strings. Apply it only when the bind address is loopback. While in the file, drop `0.0.0.0` from `isLocalOrigin` and reconsider the `X-Requested-With` bypass. Port `symaira-seek/internal/server/server.go:76-120` for consistency rather than inventing a third variant.
  - **Effort/Impact:** Low effort / high impact. Under an hour, fully reversible, no new dependency, and it closes a read-path hole that our CORS and auth layers do not cover. Confidence high — the mechanism is visible, documented and already proven inside our own ecosystem.

- [ ] **[Architecture] Run the existing retrieval benchmark harness in CI as a report-only weekly job**
  - **Status quo:** `symaira-memory/internal/bench/` is a complete evaluation harness — `corpus.go` (deterministic fixture corpus), `metrics.go`, `harness.go`, `longmemeval.go` — reachable only through `cmd/bench.go` by hand. Nothing in `.github/workflows/ci.yml` invokes it, so retrieval quality has no regression signal: a change to BM25 weighting, quantization, or scoring can silently degrade NDCG and nobody learns until a user notices. This directly blocks open issue **#391 "Serve hybrid fused retrieval instead of a single arm per query"** (priority: high) — fusion is precisely the kind of change whose value can only be argued from before/after numbers, and #393 (JSON-schema-constrained LLM responses) has the same measurement problem. Upstream `tirth8205/code-review-graph` closes this loop: `code_review_graph/eval/runner.py:29` registers seven named benchmarks, `code_review_graph/eval/configs/*.yaml` pin each corpus to an exact commit SHA with a validator that refuses a config whose pin drifts from its latest test commit (`runner.py:48-63`), and `.github/workflows/eval.yml` runs the two smallest corpora weekly, uploads result CSVs for 90 days and writes a diff table into the job summary. Its header comment states the motivation and the deliberate restraint: report-only, `|| true`, no gate on main until enough baseline history exists to set thresholds.
  - **Proposed solution:** Pattern adoption. Add `.github/workflows/eval.yml`: weekly cron plus `workflow_dispatch`, running `symmemory bench` over the built-in `DefaultCorpus`, writing results as a CSV artifact and rendering a summary table into `$GITHUB_STEP_SUMMARY`. Copy CRG's discipline, not just its plumbing — **report-only from day one**, no failure threshold until several runs of baseline exist. Two upstream details worth taking: the config validator that keeps a benchmark snapshot deterministic (ours is already deterministic via `DefaultCorpus`, so this applies if LongMemEval corpora are ever pinned), and running only the cheapest corpora so the job stays under a sane timeout.
  - **Effort/Impact:** Low-to-medium effort / high impact. The harness already exists and is tested; this is workflow wiring plus a CSV/summary emitter on `cmd/bench.go`. Fully reversible (delete one workflow file), no production code touched, and it turns #391 from a judgement call into a measured one.

## Considered and rejected

- **Cross-language schema-version parity CI job** (`.github/workflows/ci.yml:52` `schema-sync`, which fails CI when the Python and VS Code schema constants diverge) — gate 3 (Better): `cmd/root.go` passes a literal `1` to `versionkit.New`, but the SwiftUI GUI declares no matching `expectedSchemaVersion` constant, so there is no second literal to drift against. Real finding for `symaira-scope` and `symaira-seek`, not here. Revisit if the GUI ever gains a handshake constant.
- **Staleness/provenance attached to tool results** (`code_review_graph/tools/_common.py:60` `graph_provenance` — returns build time, branch, built-at SHA and `head_matches_build` so the agent knows the index is behind HEAD) — gate 3 (Better): memories are authored facts, not a derived index of a mutating source tree, so "the store is behind HEAD" is not a defect class we have. No recorded pain point. Also thin upstream: a single call site.
- **`hybrid_search` with a graceful degradation fallback** (`eval/benchmarks/search_quality.py:20`) — gate 2 (New): `internal/db/search.go` plus the hybrid scoring path already implement this, with quantization support CRG lacks.
- **Standardised error-response envelope** (`tools/_common.py:22` `_error_response`) — gate 2 (New): `internal/mcp/handlers.go` already emits structured JSON errors with stable codes (`writeJSONError`, `CodeForbidden`).
- **Bandit security-lint CI job** (`.github/workflows/ci.yml:38`) — gate 1 (Transferable): Python-specific. The Go analogue (`govulncheck`, `golangci-lint`) is already in our pipeline and `.golangci.yml`.
- **Multi-platform installer that writes MCP config for 14 AI clients** (`code_review_graph/skills.py`, `code_review_graph/uninstall.py`) — gate 4 (Worth it): `symaira-scope` already owns MCP client discovery and `mcp add/rm` for this ecosystem; duplicating that surface in symaira-memory adds a second writer to the same user files for no gain.
- **Five translated READMEs (zh/ja/ko/hi)** — gate 4 (Worth it): translation maintenance for a solo repo with no measured non-English demand. Scale-fit failure.
- **Discord community + trendshift badges + issue-template scaffolding** — gate 4 (Worth it): 27.7k-star inbound-contribution infrastructure; our contribution volume does not justify the upkeep.
- **VS Code extension shipped in-repo** (`code-review-graph-vscode/`) — gate 4 (Worth it): we already ship a native SwiftUI GUI plus a browser extension; a third client surface is cost without a stated user.

## Open questions

- CRG's README claims 38×–528× token reductions across six repos, but the pinned-corpus results in `evaluate/results/` are CSVs without a reproducibility statement of the raw-context baseline. Whether their token-efficiency methodology is worth mirroring in our own `docs/research` figures cannot be judged from the outside — settling it would need reading `eval/token_benchmark.py` against one of their published result CSVs and reproducing a single row.
- CRG's weekly eval is explicitly not a gate ("until the co-change baseline has enough history to set thresholds"). How many runs of history they consider enough is not recorded anywhere in the repo; we would be picking that number blind. Three to four weeks of our own NDCG variance would settle it empirically.

**First step:** add the `Host`-header middleware to `internal/mcp/http_server.go` — it is under an hour, closes a read-path hole, and the exact Go shape already exists in `symaira-seek/internal/server/server.go:100`.

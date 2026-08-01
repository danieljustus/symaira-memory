<!-- review: timestamp=2026-07-30T12-50-13Z  repo=danieljustus/symaira-memory  head=ffc636abd728277670ad5377767f721e81b37438 -->
<!-- adopt: source=writerslogic/holographic-memory  source_ref=76b5955d6df0f9dc1610b60144e47e2b9eff28cc  source_url=https://github.com/writerslogic/holographic-memory  depth=clone  license=Apache-2.0 -->

# Adoption Report — danieljustus/symaira-memory ← writerslogic/holographic-memory — 2026-07-30

## Sources

| Field | Value |
|---|---|
| SOURCE | `writerslogic/holographic-memory` (https://github.com/writerslogic/holographic-memory) |
| Ref analyzed | `76b5955` (main) |
| Language / License | Rust (N-API Node.js binding) / Apache-2.0 |
| Health | 9 stars, last push 2026-07-27, active, latest release v0.6.0 (2026-06-21), ~5 MB |
| Scope | all facets, full clone |
| TARGET | `danieljustus/symaira-memory` @ `ffc636a` |

Note: SOURCE is a Vector Symbolic Architecture (hyperdimensional computing) memory engine in Rust. TARGET is CGO-free Go — nothing transfers as code or dependency; all findings are pattern-level (license-compatible anyway). SOURCE health caveat: small project (9 stars, single org), but it is benchmark-driven with committed result artifacts and a documented research log, so its claims are unusually well-evidenced for its size.

## Verdict

Worth learning from, narrowly: not for its core substrate (VSA/BSC hypervectors replace rather than augment our Ollama+FTS5 hybrid, and their own capacity log documents the floors), but for how it *measures* and *cuts* retrieval. The highest-value takeaway is their planted-fact recovery-wall benchmark methodology — it maps directly onto our open issue #402 (context-pressure canaries). Secondary transfers: their entmax-based adaptive result-set cutoff (a principled replacement for fixed top-k + global absolute threshold) and property-based testing of bitwise vector invariants.

## What we already do as well or better

- Two-stage retrieval (coarse prefilter → exact rescore) → we already have binary quantization + Hamming prefilter + cosine rescore in `internal/db/binary.go:20-60`; their Hopfield cleanup stage is the same idea.
- Multi-hop graph traversal → `internal/db/entity_relations.go:349` `GraphNeighbors(entityID, depth)` BFS with temporal (`GraphNeighborsAsOf`) and provenance variants covers reachability; their algebraic multi-hop adds noise tolerance we have no recorded pain for.
- Audit logging → `internal/db/audit.go:20-35` (richer fields than their fixed-layout log; theirs is only additionally *signed*).
- Ingest admission control → covered by today's RetainDB adopt report (low-signal ingest gating), not re-reported.
- Deterministic embeddings → our FNV-1a hash fallback is already deterministic; their AtomMemory seeding adds nothing.
- Dependency-vulnerability CI → `.github/workflows/ci.yml` `govulncheck` job; their `dependency-review.yml`/`scorecard.yml` add badge posture, not a defect class.
- Release notes → GoReleaser (`.github/workflows/release.yml:55`) generates notes from conventional commits; their git-cliff setup is equivalent.
- BM25/lexical channel → our FTS5 BM25 already ships; their `idf.rs`/sparse inverted index is the same channel.

## Findings

- [ ] **[Architecture] Add a planted-fact recovery-wall benchmark mode to the bench harness**
  - **Status quo:** `internal/bench/harness.go:16-55` measures Recall@k/NDCG/MRR on fixed corpora (builtin + LongMemEval) but never varies corpus size, so we have no canary for *when* retrieval degrades as stored memories accumulate — exactly the gap named in open issue #402 ("canary-recall, bounded-output, source-recovery release gate", surfaced in the 2026-07-30 architecture review). Upstream `writerslogic/holographic-memory` runs a dedicated scaling suite (`src/bin/hms-scaling.rs:1-60`): plant N known facts at increasing N, measure the recall "hard wall" (95% recall cutoff), commit results as JSON artifacts (`bench_local_16384_256.json` etc.), and keep a running campaign log of what moved the wall (`docs/CAPACITY-CAMPAIGN-LOG.md`).
  - **Proposed solution:** Pattern adoption (no code copy). Extend `internal/bench` with a `--scaling` mode: synthesize corpora at geometric sizes (e.g. 1k/4k/16k/64k memories), plant a fixed set of known retrievable facts per size, and report the N at which Recall@k drops below 0.95 for BM25, vector, and hybrid modes. Emit JSON so the weekly bench CI job (#399) can diff walls over time and fail on regression. Skip their scaling-law curve fitting; the wall number per mode is the canary.
  - **Effort/Impact:** Low effort / high impact. Directly answers #402 and feeds #399; fully reversible (additive bench mode, no production-code change).

- [ ] **[Performance] Replace fixed top-k + absolute min-score cutoff with a distribution-relative sparse cutoff (sparsemax/entmax)**
  - **Status quo:** `internal/db/hybrid.go:211-216` truncates fused results at a caller-supplied fixed `limit`, and `internal/db/minscore.go:5-17` filters by an absolute global threshold — so a query with one strong hit and nine noise items still returns ten items unless the global threshold happens to catch the tail, and abstention tuning is a single constant for all score distributions (bench-side pain visible in `internal/bench/abstention_test.go`). Upstream `writerslogic/holographic-memory` applies α-entmax over the score vector (`src/core/hopfield.rs:60-95`): scores whose mass falls below the sparse-attention cutoff get exact zeros, so the result set sizes itself per query and "no relevant result" emerges naturally as an empty set; motivation documented in-file (Tsallis α-entropy, Santos et al. JMLR 2025) and wired into their main query path (`src/core/engine/query.rs`).
  - **Proposed solution:** Pattern adoption (no code copy). Add a small pure-Go sparsemax (α=2) function over the fused score vector in `internal/db/hybrid.go`: keep results with non-zero weight, still capped by `limit` as a hard bound (upstream does the same, `src/core/hopfield.rs:95`). Keep `FilterByMinScore` as an opt-in absolute floor. Validate on the existing abstention benchmark before changing any default.
  - **Effort/Impact:** Low-medium effort / medium-high impact. Improves precision of the default retrieval path and gives abstention (#391-adjacent) a principled, per-query cutoff; reversible (flag-gated, old path stays).

- [ ] **[Architecture] Property-based tests for quantization, Hamming and sync-oplog invariants**
  - **Status quo:** `internal/db/binary.go:20-60` (sign-bit packing, LSB-first word layout), `internal/db/sync_oplog.go` (oplog serialization/tombstones) and `internal/db/lsh.go` are covered by example-based tests only — a bit-order or boundary regression is caught only if someone wrote that exact case. Upstream `writerslogic/holographic-memory` uses proptest for vector-algebra invariants (`src/lib_proptest.rs:1-20`, wired via `src/lib.rs:2937`, `proptest = "1.0"` in `Cargo.toml`) — their usage is thin (one property), but the pattern is standard and the defect class is independently obvious on our side: bitwise packing and serialization code has algebraic invariants (round-trip identity, symmetry, determinism) that example tests systematically under-cover.
  - **Proposed solution:** Pattern adoption. Add `pgregory.net/rapid` (pure Go, MIT, no CGO — constraint-compliant) as a test-only dependency and write properties for: `BinarizeVector` determinism and popcount bounds, `HammingDistance` symmetry and `d(a,a)=0`, oplog encode→decode round-trip, LSH band-hash determinism. Keep the existing example tests; properties complement them.
  - **Effort/Impact:** Low-medium effort / medium impact. Removes a defect class in the bit-level and sync code paths; one small, swappable test-only dependency; reversible (delete the properties, drop the dep).

## Considered and rejected

- **Full VSA/BSC substrate (XOR-bind/majority-bundle hypervectors as memory representation)** — gate 3 (Better): replaces rather than augments our Ollama dense + FTS5 hybrid; no TARGET-side retrieval bottleneck it demonstrably fixes, and their own campaign log (`docs/CAPACITY-CAMPAIGN-LOG.md`) documents superposition floors (~0.03–0.06·D) for the codes that would fit our constraints.
- **Fuzzy structural query / role-filler algebra & analogies (A:B :: C:?)** — gate 3 (Better): symbolic `GraphNeighbors` depth-BFS already covers reachability; noise-tolerant algebraic inversion and analogical reasoning have no recorded pain point or open issue on our side.
- **GapDetector / HypothesisEngine (epistemic gap discovery over relation profiles, `src/core/cognition/gaps.rs`)** — gate 3 (Better): interesting, but our entity graph is young and `internal/consolidation/linker.go` already does similarity-based linking; no recorded pain asking for missing-relation surfacing.
- **Ternary hypervectors (sign-bit ternary quantization, `src/core/ternary.rs`)** — gate 3 (Better): barely wired upstream (trait impl + own module only, no engine call sites), and no measured prefilter-recall deficit on our side to justify a second quantization format.
- **COSE Sign1 / W3C VC / C2PA provenance stack (`src/core/provenance/`)** — gate 4 (Worth it): we would own a large crypto surface (Ed25519 envelopes, JCS canonicalization, C2PA manifests) with no recorded tamper-evidence pain; device sync already runs AES-256-GCM.
- **Signed fixed-layout audit log (`src/core/audit.rs`)** — gate 3 (Better): `internal/db/audit.go` covers the operational need; optional Ed25519 entry signing adds crypto surface without a demand signal.
- **Admission control (`src/core/admission.rs`)** — gate 2 (New): already covered by today's RetainDB adopt report (low-signal ingest gating).
- **Hopfield attractor cleanup as a retrieval stage** — gate 2 (New): our Hamming prefilter + full-cosine rescore in `internal/db/binary.go` is the same two-stage idea.
- **Scorecard / dependency-review CI workflows** — gate 3 (Better): the `govulncheck` job in `.github/workflows/ci.yml` already covers the vulnerability defect class; OpenSSF Scorecard adds posture badges, not defects prevented.
- **git-cliff changelog generation (`cliff.toml`)** — gate 2 (New): GoReleaser (`.github/workflows/release.yml:55`) already generates release notes from our conventional commits.
- **Federated queries across instances** — gate 3 (Better): no demand; our device-sync relay already consolidates instances into one store rather than querying across them.
- **JSON-schema artifacts for public types (`schemas/*.json`)** — gate 3 (Better): their schemas constrain API wire types, not LLM output; #393 needs provider-side constrained decoding, and our consolidation prompt already embeds its schema (`internal/consolidation/engine.go:462-464`).
- **Deterministic seeding for reproducible embeddings (AtomMemory)** — gate 2 (New): our FNV-1a hash fallback embedding is already deterministic.

## Open questions

- Does the entmax cutoff actually improve precision on *our* score distributions (Ollama cosine fused with BM25)? Settle with the abstention benchmark in `internal/bench/abstention_test.go` before flipping any default.
- What corpus size does our SQLite+Hamming pipeline realistically reach before the vector channel degrades — i.e., is a recovery wall even reachable at self-hosted scale (10³–10⁵ memories)? The scaling bench mode (finding 1) is precisely what answers this; until it exists, finding 2's impact is partly hypothetical.
- Upstream's entmax is applied to Jaccard-over-sparse-binary scores, not dense cosine; the transformation is score-agnostic in principle, but the α/β parameters will need re-tuning for our score range.

Best first step: build the planted-fact recovery-wall mode in `internal/bench` — it answers #402, needs no production-code change, and produces the data that decides finding 2.

---
*Housekeeping: `docs/adopt/` is not currently git-ignored — consider adding a `docs/adopt/` entry to `.gitignore` (suggested, not written).*
*Prompt-injection note: `conductor/` in the SOURCE contains prompts addressed to their own multi-model debate agents (e.g. `conductor/final_prompt.md`: "Synthesized from a 5-round, 36-turn debate across Claude Opus 4.8, DeepSeek V4-Pro, MiMo V2.5-Pro, Step 3.7 Flash, MiniMax M3, and Qwen 3.7 Max"). These are instructions for their orchestration, treated as data; nothing in the SOURCE was obeyed. No text addressed to the reading agent was found.*

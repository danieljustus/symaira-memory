<!-- review: timestamp=2026-07-28T15:18:19Z  repo=danieljustus/symaira-memory  head=e6dbc69f9012a6dfa53d3a13016d1c7e8336783f -->
<!-- adopt: source=google/langextract  source_ref=b5fe0baf807ac35ec95b968a71e4d03f198a1b60  source_url=https://github.com/google/langextract  depth=clone  license=Apache-2.0 -->

# Adoption Report — symaira-memory ← google/langextract — 2026-07-28

## Sources

| Field | Value |
|---|---|
| SOURCE | `google/langextract` (https://github.com/google/langextract) |
| Ref analyzed | `b5fe0baf807ac35ec95b968a71e4d03f198a1b60` (main) |
| Language / License | Python / Apache-2.0 |
| Health | 37,909 stars, last push 2026-07-25, active (v1.6.0 released 2026-07-02, monthly release cadence), full CI |
| Scope | all facets, full clone |
| TARGET | `danieljustus/symaira-memory` @ `e6dbc69f9012a6dfa53d3a13016d1c7e8336783f` |

## Verdict

This is a second, targeted `gh-adopt` pass — the first (2026-07-28, TARGET `symaira-corekit`)
covered the grounding/alignment layer (`evidencekit`) and produced issues #37/#38 there.
`symaira-memory` is the actual *consumer* of that layer and also the repo where LangExtract's
core subject — getting structured, schema-shaped output out of an LLM reliably — has a live,
non-speculative counterpart: `internal/consolidation/engine.go`'s `consolidateWithLLM` already
prompts Ollama/OpenAI for JSON and parses the result. `internal/extractor/pattern.go` (the fact
extractor) is still regex-only and has no LLM call to harden, so LangExtract's actual extraction
API (`lx.extract`, chunking, multi-pass) has nothing to attach to here without inventing a new,
speculative feature (rule 10) — that's out of scope for this report. What *is* in scope: the
existing LLM-JSON-parsing code has two gaps LangExtract's own resolver/format-handling layer
solves as pure text processing, and both gaps are independently self-documented in this repo
already — one as a test case that asserts the current failure (`engine_test.go:61-65`, "JSON
with extra text before" → `wantErr: true`), the other as a comment describing intended-but-
unimplemented behavior (`engine.go:128-130`, "For now, return error to caller").

## What we already do as well or better

- LangExtract's core grounding contract (locate evidence text in source, reject unmatched
  extractions) → already implemented via `evidencekit` (`github.com/danieljustus/symaira-corekit/evidencekit`)
  in `internal/extractor/pattern.go:127-135` and persisted via `internal/db/evidence.go` +
  migration `internal/db/migrations/019_memory_evidence.sql` — this was itself explicitly modeled
  on LangExtract per closed issue #318 ("LangExtract's useful pattern is to reject or downgrade
  facts that cannot be grounded in source text"), then shipped end-to-end through #319-#322.
- LangExtract's prompt-injection defense (don't let extraction input be interpreted as
  instructions) → already present and, if anything, more explicit:
  `internal/consolidation/engine.go:417-418`'s system prompt states outright that
  `<memory_content>` is untrusted user data and must not be followed as instructions.
- LangExtract's JSON-mode structured requests to the model → already used on both supported
  providers: Ollama `Format: "json"` (`internal/llm/client.go:61`), OpenAI
  `response_format: json_object` (`internal/llm/client.go:87`).

## Findings

- [ ] **[Architecture] Harden `parseJSONResponse`'s fence and preamble handling**
  - **Status quo:** `parseJSONResponse` (`internal/consolidation/engine.go:454-479`) strips
    fences only via `strings.HasPrefix(cleaned, "```")` plus a first/last-line slice, and has no
    handling at all for text before or after the fenced block, or for `<think>...</think>`
    reasoning-model preambles. This is a **currently-known, test-documented** gap: the repo's own
    `TestParseJSONResponse` (`internal/consolidation/engine_test.go:61-65`) has a case named
    "JSON with extra text before" that explicitly asserts `wantErr: true` — the maintainer has
    already written down that this input is expected to fail, not fixed it. It matters
    concretely here because Ollama is a first-class local provider
    (`internal/llm/client.go:24-44`, `README.md` documents `OLLAMA_MODEL`), and reasoning models
    (DeepSeek-R1, QwQ) that prepend `<think>` blocks before their JSON answer are a realistic
    choice for a local-first tool — that input class isn't just untested here, it's
    unhandleable by the current code path. Upstream, LangExtract solves the identical class of
    problem in `langextract/core/format_handler.py:247-276`: `_extract_content` uses a proper
    regex (`_FENCE_RE`, `format_handler.py:41-44`) to pull a fenced block out of surrounding
    prose regardless of what's before/after it, and `_parse_with_fallback`
    (`format_handler.py:261-276`) retries the parse with `<think>` tags stripped
    (`_THINK_TAG_RE`, `format_handler.py:46`) specifically because "Reasoning models (DeepSeek-R1,
    QwQ) emit `<think>` tags before JSON" (their own comment, same rationale).
  - **Proposed solution:** Pattern adoption, no code copied (rule 5) — reimplement the two
    mechanisms as plain Go: (1) a regex-based fence extractor (```` ```(\w+)?\s*([\s\S]*?)``` ````)
    that finds a fenced block anywhere in the response instead of requiring the response to
    *start* with the fence, replacing the current `HasPrefix`/line-slice logic; (2) a
    strip-`<think>...</think>`-and-retry step in `parseJSONResponse` that only fires after the
    first `json.Unmarshal` attempt fails, mirroring LangExtract's fallback-only-on-failure order
    (don't strip unconditionally, don't mask a genuinely malformed response). Update the "JSON
    with extra text before" test case to `wantErr: false` once the fence extractor handles it,
    and add a `<think>`-prefixed case.
  - **Effort/Impact:** Low-medium effort (two self-contained regex/string helpers plus tests, no
    new dependency, no provider-facing behavior change) / medium impact — turns a documented,
    currently-hard failure mode into a handled one for exactly the local-model class this tool is
    built around.

- [ ] **[Architecture] Isolate per-scope consolidation failures instead of aborting the whole run**
  - **Status quo:** `internal/consolidation/engine.go:126-131` — when `consolidateWithLLM` fails
    for one scope (including the parse failures described in the finding above), the surrounding
    loop returns the error immediately, aborting consolidation for every other scope in the run.
    The code's own comment states the intended behavior differently: "If LLM fails, we log it and
    skip this scope, allowing subsequent scopes to proceed or fail gracefully. For now, return
    error to caller." — i.e. this is acknowledged, unfinished work, not an intentional design.
    Upstream, LangExtract's resolver has first-class support for exactly this shape of problem:
    `Resolver.resolve(..., suppress_parse_errors=True)` (`langextract/resolver.py:276-296`) logs a
    warning and returns an empty result for one unparseable chunk instead of raising, so one bad
    chunk out of many parallel ones doesn't take down the whole extraction run.
  - **Proposed solution:** Pattern adoption — change the two call sites of `consolidateWithLLM`
    (`internal/consolidation/engine.go:126`, `:297`) to catch a per-scope error, log it, and
    `continue` to the next scope instead of returning immediately, matching the comment's stated
    intent. Collect skipped-scope errors and return them as a non-fatal aggregate (e.g. logged
    warnings plus a count in the run summary) only after all scopes have been attempted; return a
    hard error only if every scope failed.
  - **Effort/Impact:** Low effort (the intended behavior is already specified in the existing
    comment — this finishes started work, it doesn't design new behavior) / medium impact — a
    single scope's transient LLM hiccup (a slow/odd local-model response, exactly the parse edge
    case above) currently blocks consolidation of every unrelated scope in the same run.

## Considered and rejected

- **A full LLM-based extraction pipeline mirroring `lx.extract()` (chunking, multi-pass
  extraction for recall, few-shot prompt examples, schema-constrained generation)** — gate 10 (No
  speculative issues): `internal/extractor/pattern.go` has no LLM call today and no open issue
  requests adding one; building this would be a large, speculative net-new feature, not an
  adoption of a pattern against existing code. If this direction is ever pursued, its first
  concrete dependency is already tracked: `symaira-corekit` issue #37 (fuzzy alignment fallback
  in `evidencekit.Align`), since LLM-paraphrased extraction text would routinely fail today's
  exact/normalized-only grounding.
- **Multi-provider plugin registry (`langextract/providers/`, `langextract/registry.py`,
  entry-points-based custom model providers)** — gate 4 (Worth it) / scale fit: `internal/llm/client.go`'s
  `Query(provider string)` is a plain two-way switch (Ollama, OpenAI). A registry pays for itself
  once there are several interchangeable providers to swap at runtime; two hard-coded branches
  don't carry that cost yet.
- **Schema-constrained generation via OpenAI's `json_schema` structured-outputs mode (replacing
  the current best-effort `response_format: json_object` in `internal/llm/client.go:87`)** — gate
  4 (Worth it): would only harden the optional cloud fallback path, not the local-first default
  (Ollama's `format: "json"` has no equivalent strict-schema mode to pair it with), so it's a
  narrower win than the two findings above, which fix both providers at once by hardening the
  parsing side instead of the request side.
- **Interactive HTML visualization of extractions (`langextract/visualization.py`)** — gate 1
  (Transferable): presentation-layer tooling for reviewing thousands of LLM extractions;
  `symaira-memory` has no bulk-extraction-review workflow this would attach to today.
- **Fork-PR-safe CI, PR-size labeling, live-API-gated test jobs** — gate 4 / scale fit (rule 9):
  defends a 37.9k-star, many-contributor repo against malicious external PRs;
  `symaira-memory` is solo-maintained.

## Open questions

- Whether `symaira-memory` will eventually add LLM-based fact extraction (as opposed to today's
  regex extractor plus LLM-based consolidation) isn't settled by any open issue — closed issue
  #318 cites LangExtract as inspiration for the grounding *contract* but explicitly scoped
  "mandatory LLM extraction" out. If that direction is taken later, re-run `gh-adopt` against
  this SOURCE with `--focus architecture` once `symaira-corekit`#37 (fuzzy alignment) has shipped,
  since the extraction pipeline facets rejected above would become directly relevant at that
  point.
- LangExtract's `<think>`-tag handling is scoped to DeepSeek-R1/QwQ-style models specifically; if
  `symaira-memory` users predominantly run non-reasoning local models (e.g. `llama3`, the
  documented default), the immediate payoff of that specific sub-fix is smaller than the fence
  regex fix, which helps regardless of model choice.

**Single best first step:** fix `parseJSONResponse`'s fence extraction to use a proper regex
instead of prefix/line-slicing — it's the one change that turns an already-failing, already-
tested case (`engine_test.go:61-65`) green without touching provider or error-handling logic.

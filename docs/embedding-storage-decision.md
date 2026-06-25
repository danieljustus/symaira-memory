# Embedding Storage Decision

Status: accepted for planning, no Memory production code in this change.
Date: 2026-06-25
Related issue: https://github.com/danieljustus/symaira-memory/issues/259

## Decision

Symaira Memory should not add TurboQuant production code yet. The next
production candidate is a JSON-to-BLOB embedding storage migration, measured
against the current JSON text representation before any quantized sidecar work
is considered.

The current `memories.embedding` column stores JSON-encoded `[]float32` values.
That format is simple and portable, but it is larger and slower to decode than
a little-endian `float32` BLOB. Memory's workload is usually a personal or
project-local memory store, not Seek's higher-volume document chunk index. A
binary float32 storage cleanup is therefore the lower-risk first optimization:
it keeps exact search quality, preserves the existing LSH and hybrid search
model, and can be made backward-compatible for sync, backup, and restore.

TurboQuant remains a deferred option. Revisit it only after Memory has measured
the JSON-to-BLOB path and can show that large local stores still need a
quantized search path.

## Seek Evidence Reviewed

The Seek TurboQuant work is complete enough to use as input:

- https://github.com/danieljustus/symaira-seek/issues/175 closed via
  https://github.com/danieljustus/symaira-seek/pull/181.
- https://github.com/danieljustus/symaira-seek/issues/176,
  https://github.com/danieljustus/symaira-seek/issues/177, and
  https://github.com/danieljustus/symaira-seek/issues/178 closed via
  https://github.com/danieljustus/symaira-seek/pull/182.

Seek's benchmark result for 768-dimensional vectors reported:

| Mode | Bytes/vector | Compression | Direct recall@10 | Exact rerank recall@10 |
| --- | ---: | ---: | ---: | ---: |
| float32 | 3072 | 1.0x | 1.0000 | n/a |
| 2-bit | 256 | 12.0x | 0.5850 | 0.9500 |
| 3-bit | 384 | 8.0x | 0.7850 | 1.0000 |
| 4-bit | 512 | 6.0x | 0.9000 | 1.0000 |

The keep recommendation was conditional: 4-bit direct recall is usable only
with care, and exact cosine rerank over an oversampled shortlist is what
recovers quality.

Seek's implementation shape also matters for Memory:

- Quantized search is opt-in, with `vector_quantization = "off"` by default.
- Quantized sidecars are nullable BLOB/TEXT columns; the original float32
  embedding remains authoritative.
- Sidecar metadata is versioned and includes dimension, bit width, quantizer
  mode, projection seed, and norm.
- The codec supports 2-bit, 3-bit, 4-bit, and channel-split prototype modes,
  but the operator-facing config exposes 2, 3, and 4 bits.
- Missing, stale, or incompatible sidecars fall back to exact search.

Those constraints are good safety rails, but Memory should not copy Seek's
internal package directly. If a future Memory benchmark justifies quantization,
the preferred path is a shared, public core package after the Seek API has had
time to settle.

## Memory-Specific Constraints

Any embedding storage change must preserve these Memory behaviors:

- Standalone-first operation with no dependency on another Symaira binary.
- CGO-free Go and SQLite WAL mode.
- Existing PII guard behavior.
- Existing LSH candidate filtering and hybrid BM25/vector search semantics.
- Sync and backup compatibility for current databases.
- MCP and HTTP API compatibility, including not exposing embeddings unless an
  existing endpoint explicitly requests them.
- Exact search quality unless a later opt-in feature documents and measures
  a recall trade-off.

## Migration Plan If JSON-to-BLOB Proceeds

1. Add a nullable binary embedding column, for example `embedding_blob BLOB`,
   while leaving the current JSON `embedding` column untouched.
2. Add a small codec helper that encodes and decodes `[]float32` as
   little-endian float32 bytes. Validate that decoded dimensions match
   `embedding_dim`.
3. Update read paths to prefer the binary column when present and fall back to
   JSON for old rows.
4. Update write paths to write both formats for one compatibility release, or
   write binary plus JSON fallback until backup and sync readers are updated.
5. Add an idempotent backfill command or migration routine for existing rows.
6. Update backup and restore tests with old JSON-only rows, mixed rows, and new
   binary rows.
7. Only after the compatibility window, consider whether the JSON column can be
   deprecated or retained as an interchange fallback.

This plan intentionally keeps `embedding_dim`, `embedding_source`,
`embedding_model`, and `lsh_hash` semantics unchanged.

## Benchmark Plan Before Production Changes

Before implementing the migration, add a Memory-specific benchmark that can be
run from a clean checkout and reports:

- Search latency for current JSON decode versus binary decode.
- Database size with representative counts, at least 100, 1,000, 10,000, and
  100,000 memories when practical.
- Backup export size and restore time.
- Recall and ranking quality against the current exact Memory search.
- LSH candidate count and hybrid search behavior, so binary storage does not
  accidentally change prefilter semantics.
- Backfill time for existing JSON rows.

The benchmark should include deterministic lexical fallback embeddings and, when
available locally, an Ollama-backed fixture. It should avoid network dependence
and should not require Pro or cloud services.

## TurboQuant Revisit Criteria

Open a follow-up TurboQuant implementation issue only if all of these are true:

- JSON-to-BLOB has already been measured or implemented and still leaves a
  meaningful storage or latency problem for large Memory stores.
- The Memory benchmark includes a dataset large enough for approximate search
  to matter.
- A shared, license-compatible vector quantization package exists in a public
  repo, or Memory has a clear reason to host its own original implementation.
- The feature remains opt-in, exact rerank is enabled by default, and missing or
  stale sidecars fall back to exact search.
- Backup, restore, sync, and MCP/API compatibility are covered by tests.

Until those criteria are met, Memory should keep exact embeddings as the source
of truth and avoid TurboQuant production code.

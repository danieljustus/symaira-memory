## What's changed

### Features
- #366 Binary vector quantization with Hamming prefilter for search — closes #360
- #365 Temporal validity for entity relations (valid_from/valid_until, as-of queries) — closes #359
- #363 Delete propagation and encrypted relay mode for sync — closes #361
- #364 Working-memory tier with TTL eviction and consolidation handoff — closes #358
- #367 LongMemEval corpus support and abstention threshold for memory_search

### Fixes
- Restore missing sections in Apache 2.0 LICENSE file
- Bump modernc.org/sqlite from 1.53.0 to 1.54.0 (#368)
- Fix gofmt formatting in test files

### Tests & Quality
- #372 Coverage improvements: sync-relay, service wrapper, working-memory consolidation
- #384 Coverage tests for 10 packages (importer packages, cmd, working-memory)
- #385 Coverage tests for memorytool importer
- Overall coverage improved from 68.5% to 79.7% (+11.2%)

### Closed Issues
- #343 Deterministic entity candidate resolution
- #344 ID-based, provenance-aware entity relations
- #348 Fail-closed role enforcement for tokens
- #349 Synchronize mutable JWTProvider state
- #350 Deduplicate parallel auth/CORS middleware
- #351 Unify --output vs --format flag split
- #352 Accept memory content as positional argument in symmemory set
- #353 Remove dead in-memory BM25 index
- #354 Push embedding-source and archival filters into candidate SQL
- #358 Working-memory tier with TTL eviction
- #359 Temporal validity for entity relations
- #360 Binary vector quantization with Hamming prefilter
- #361 Delete propagation and encrypted relay mode
- #369 Coverage regression from v0.14.0
- #373–380, #382–385 Coverage improvements for importers, cmd, working-memory, memorytool

**Full Changelog**: https://github.com/danieljustus/symaira-memory/compare/v0.14.0...v0.15.0

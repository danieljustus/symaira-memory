## What's Changed

### Features
- #164 Context assembler with token-budget progressive retrieval — closes #157
- #164 Composite retrieval ranking (relevance × recency × importance) — closes #158
- #164 Pure-Go BM25 hybrid search with RRF fusion — closes #159
- #164 Audit log, TTL purge, and `symmemory purge` command — closes #160
- #164 Eval harness `symmemory bench` command — closes #161
- #164 Temporal validity windows and fact supersession — closes #162
- #168 Cursor-based pagination for sync/changes endpoint — closes #165

### Fixes
- #168 Fix BM25 index concurrency for MCP server — closes #167

### Security
- #168 Harden sync/apply endpoint against malicious memory IDs — closes #166

### Docs
- #163 Archive Symmemory research report and figures

### Tests
- #155 Add test coverage for db, consolidation, MCP, and memory packages — closes #151, #152, #153, #154

### Closed Issues
- #151 Test coverage for db package
- #152 Test coverage for consolidation engine
- #153 Test coverage for MCP server
- #154 Test coverage for memory package
- #157 Context assembler: wire up token-budget progressive retrieval
- #158 Retrieval ranking: add composite scoring
- #159 Hybrid retrieval: fuse BM25 keyword search with vector similarity
- #160 Governance: retention policies, session TTL auto-purge, and audit log
- #161 Eval harness: measure token-reduction and retrieval-quality KPIs
- #162 Temporal memory: validity windows and fact supersession
- #165 Add pagination to sync/changes endpoint
- #166 Harden sync/apply endpoint against malicious memory IDs
- #167 Fix BM25 index concurrency for MCP server

**Full Changelog**: https://github.com/danieljustus/symaira-memory/compare/v0.6.0...v0.6.1

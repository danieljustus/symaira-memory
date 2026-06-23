## What's Changed

### Fixes
- #230 Doctor and TUI dashboard now validate Ollama embeddings endpoint with a real POST instead of a lightweight GET, avoiding false negatives — closes #221
- #230 `set` and `import` honor `--output json` — closes #222
- #230 OpenMemory, Mem0 and ChatGPT memory-tool importers wired into the import CLI — closes #223
- #230 Explicit unknown `--tool` requests return a non-zero error naming the importer — closes #224
- #230 Backup exports require an encryption password; unencrypted export path removed — closes #225
- #230 MCP `memory_get` and `memory_search` return compact responses that omit embedding vectors — closes #226
- #230 MCP `memory_set` validates that metadata is a valid JSON object — closes #227
- #230 Secondary fact extraction now skips facts that duplicate the primary memory content — closes #228

### Tests
- #238 Rule CRUD operations — closes #235
- #239 JWT revocation persistence across provider instances — closes #234
- #240 Package-level `Redact()` tests for PII Guard — closes #231
- #241 Table-driven tests for role-based access control — closes #232
- #242 Scope validation and project detection — closes #233
- #243 Sync cursor persistence — closes #236
- #249 JWT, entity, profile, and HTTP auth coverage gaps — closes #244, #245, #246, #247, #248

### Closed Issues
- #221 Doctor reports Ollama 404 even when embeddings endpoint works
- #222 `set` and `import` should honor `--output json`
- #223 Wire OpenMemory, Mem0 and ChatGPT memory-tool importers into CLI
- #224 Unknown `--tool` should return error with valid names
- #225 Backup export should require encryption password
- #226 MCP responses should omit embedding vectors
- #227 MCP `memory_set` should validate metadata JSON
- #228 Fact extraction should skip duplicate facts
- #231 Add tests for PII Guard
- #232 Add tests for role-based access control
- #233 Add tests for scope validation and project detection
- #234 Add tests for JWT revocation persistence
- #235 Add tests for rule CRUD operations
- #236 Add tests for sync cursor persistence
- #244 Add tests for JWT secret persistence
- #245 Add tests for JWT token revocation persistence
- #246 Add tests for entity persistence
- #247 Add tests for profile persistence
- #248 Add tests for HTTP auth enforcement

**Full Changelog**: https://github.com/danieljustus/symaira-memory/compare/v0.7.0...v0.7.1

## What's Changed

### Features
- Add `--include-embedding` flag to `search` and `get` commands — embedding vectors are now omitted from JSON output by default for cleaner agent workflows. Use `--include-embedding` to opt-in.

### Fixes
- Fix release version injection — GoReleaser now correctly injects version, commit, and date into the binary
- Fix CI timeout in benchmark tests by using `gzip.BestSpeed`
- Apply gofmt formatting fixes

### Tests
- Add comprehensive tests for doctor command health checks
- Add comprehensive tests for LLM client
- Add comprehensive tests for cross-tool memory linker

### Closed Issues
- #308 Add tests for cross-tool memory linker
- #309 Add tests for LLM client
- #310 Add tests for doctor command health checks

**Full Changelog**: https://github.com/danieljustus/symaira-memory/compare/v0.8.0...v0.9.0

## What's Changed

### Features
- **Session Importer Framework** — Pluggable architecture for importing memories from external AI coding tools (#121)
- **External Memory Tool Import** — Import from OpenMemory, ChatGPT, and Mem0 (#127)
- **Claude Code Session Importer** — Import conversation transcripts from Claude Code (#122)
- **Hermes Agent Session Importer** — Import from Hermes Agent sessions (#123)
- **Codex CLI Session Importer** — Import from OpenAI Codex CLI (#124)
- **Aider Session Importer** — Import from Aider chat history (#125)
- **Data Source Importer Framework** — Extended importer interfaces for categorization, privacy levels, and incremental imports (#135)
- **Git Importer** — Import local commit history with diff summaries (#129)
- **GitHub Importer** — Import PRs and issues via gh CLI (#130)
- **Shell History Importer** — Import zsh/bash command history with tagging (#131)
- **Google Calendar Importer** — Import calendar events as memory facts (#131)
- **Email Importer** — Import emails via Himalaya CLI (#132)
- **Obsidian/LifeOS Vault Importer** — Import notes from Obsidian vaults (#133)
- **Paperless-ngx Importer** — Import document metadata from Paperless (#134)
- **Dream CLI Command** — Run memory consolidation with `symmemory dream` (#119)
- **Consolidation Engine** — Merge and deduplicate memories automatically (#118)
- **Cross-Tool Memory Linker** — Find and link related memories across different tools (#126)

### Improvements
- Add consolidation status to memories and filter archived entries (#117)
- Add `Conn()` method to db package for direct SQL access

### Database
- New `import_state` table for tracking import progress
- New `consolidation_status` field on memories table

## Closed Issues
- #117 Add consolidation status to memories
- #118 Implement consolidation engine
- #119 Add dream CLI command
- #121 Cross-tool session import framework
- #122 Claude Code session importer
- #123 Hermes Agent session importer
- #124 Codex CLI session importer
- #125 Aider session importer
- #126 Cross-tool memory linking
- #127 External memory tool import
- #129 Git importer
- #130 GitHub importer
- #131 Google Calendar importer
- #132 Email importer
- #133 Obsidian vault importer
- #134 Paperless-ngx importer
- #135 Data source importer framework

**Full Changelog**: https://github.com/danieljustus/symaira-memory/compare/v0.4.0...v0.5.0

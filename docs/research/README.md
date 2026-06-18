# Symmemory — Research Archive

This folder archives the original research that motivated **Symaira Memory**: a
literature review and architecture proposal for a universal, self-hosted,
token-efficient AI-agent memory system.

- **[symmemory-research.md](symmemory-research.md)** — the full report (10 sections,
  tool comparison, benchmarks, architecture proposal, governance model, roadmap,
  and a sourced citation list). Figures referenced inline live in this folder.

## Status vs. implementation

Most of the proposal is already shipped in this repo (MCP + REST API + browser
extension, single Go binary on SQLite, local embeddings, PII guard, procedural
rules, scoping, encrypted backups, consolidation/"dreaming", entity linking, and
the importer framework). The gaps that remained sensible were filed as issues:

| Issue | Topic | Research section |
| :--- | :--- | :--- |
| #157 | Token-budget context assembler (wire up the summarizer) | sec04 §4.3, sec09 §9.2.3 |
| #158 | Recency × Importance × Relevance ranking (Stanford) | sec01 §1.3.3, sec04 §4.3.3 |
| #159 | Hybrid retrieval (BM25 + vector, MMR) | sec09 §9.2.2 |
| #160 | Retention policies, session TTL, audit log (GDPR) | sec07 §7.3.2/§7.3.4 |
| #161 | Eval harness for token-reduction & latency KPIs | sec01 §1.4, sec10 §10.4.1 |
| #162 | Temporal validity / fact supersession | sec02 §2.2.1, sec09 §9.2.2 |

> Provenance: deep-research output (Kimi), June 2026. The DOCX exports and the
> per-section source files were intentionally not archived — their content is
> merged into `symmemory-research.md`.

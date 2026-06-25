# Agent Instructions

This repository is the public Apache-2.0 licensed Symaira Memory self-hosted foundation.

## Ecosystem Guidance

- Before changing cross-tool integrations, shared conventions, or product
  boundaries, read `../docs/00-MASTERPLAN.md` and `../ECOSYSTEM.md`.
- Keep the standalone-first contract: this repo must build, test, and run
  without any other Symaira tool installed.

## Repository Role

- Keep this repository buildable, testable, and runnable without any private commercial code.
- Self-hosted Symaira Memory remains free and open source under the Apache-2.0 License.
- Do not add Cloud Pro, hosted-service, tenant-management, billing, subscription, customer-support, or commercial deployment code here.
- Do not add paid feature gates to the public self-hosted product.

## Relationship To Symaira Memory Pro

- The private `danieljustus/symaira-memory-pro` repository consumes this public core through versioned runtime artifacts such as containers and binaries.
- Pro must not copy this repository's source code or import `internal/` packages.
- If Pro needs a general core/runtime capability, implement it publicly here, release/tag it, then update the Pro runtime pin.
- The next planned public core target is `v0.1.0`.

## Architecture & Code Style Guidelines

- **CGO-Free Go**: All database drivers (SQLite) and vector operations (Kosinus-Ähnlichkeit) must remain 100% CGO-free for ultimate cross-platform compilation.
- **Database Safety**: Keep SQLite in WAL (Write-Ahead Logging) mode inside standard XDG directories to support simultaneous reads/writes.
- **Zero Stdio Pollution**: The MCP server transport runs over stdio. Under no circumstances must any package print to `os.Stdout` unless it is a structured JSON-RPC 2.0 message. All logs, warnings, and trace states must be safely routed to `os.Stderr` to prevent client handshake drop errors.
- **Fakt Ingestion Security**: Pre-filter all incoming memory strings through the PII Guard before committing to the SQLite database.

## Before Changing Scope

- Keep public issues focused on self-hosted/core behavior.
- Move Cloud Pro, commercial readiness, hosted compliance, tenant operations, billing, managed sync servers, and SSO/managed RBAC work to the private Pro repository.

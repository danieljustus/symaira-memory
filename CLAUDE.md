# Symaira Memory Developer Guidelines

Guidelines and commands for developers and AI agents working on this codebase.

## Build and Test Commands

- **Build binary**: `go build -o symmemory main.go`
- **Run all tests**: `go test ./...`
- **Run verbose tests**: `go test -v ./...`

## CLI Verification Cheatsheet

- **Check version**: `./symmemory version`
- **List memories**: `./symmemory list`
- **List scoped memories**: `./symmemory list --scope project`
- **Save memory**: `./symmemory set --value "My fact content" --scope global`
- **Search memories**: `./symmemory search "search keyword"`
- **List rules**: `./symmemory rule list`
- **Add rule**: `./symmemory rule add "Behavior rule text"`
- **Generate API token**: `./symmemory token generate --subject "cli-test"`
- **Verify token**: `./symmemory token verify [token]`
- **Launch TUI Console**: `./symmemory console`
- **Start MCP Server**: `./symmemory serve`
- **Start HTTP Daemon**: `./symmemory serve -p 8787`

## Code Style & Formatting

- **Go Code style**: Follow standard `gofmt` guidelines.
- **Indentation**:
  - Go source files (`.go`): Use **tabs** for indentation (tab size 4).
  - Web & Config files (`.yaml`, `.json`, `.css`, `.html`, `.sh`): Use **2 spaces** for indentation.
- **Imports order**: Standard Go grouping (stdlib block, space, external modules block).
- **Zero-CGO**: Maintain CGO-free compilations. Avoid importing packages that require C-linkers.
- **Standard Library first**: Prefer Go standard library over external dependencies where possible.

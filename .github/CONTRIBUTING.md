# Contributing to Symaira Memory

Thank you for your interest in contributing to Symaira Memory! This document provides guidelines and instructions for contributing.

## Prerequisites

- Go 1.26.3 or later
- No C compiler required (CGO-free)
- Git

## Building from Source

```bash
git clone https://github.com/danieljustus/symaira-memory.git
cd symaira-memory
go build -o symmemory main.go
./symmemory version
```

## Running Tests

```bash
go test -v ./...
```

## Code Style

This project enforces `gofmt` formatting in CI. Before submitting a PR:

```bash
# Check formatting
gofmt -l .

# Auto-fix formatting
gofmt -w .
```

Also run `go vet` to catch common issues:

```bash
go vet ./...
```

## Pull Request Process

1. Fork the repository and create a feature branch from `main`
2. Make your changes following the code style guidelines above
3. Add or update tests for any new functionality
4. Ensure all tests pass: `go test -v ./...`
5. Ensure formatting is clean: `gofmt -l .` returns empty
6. Submit a pull request with a clear description of the changes

## Commit Messages

Write clear, descriptive commit messages. Use the imperative mood ("Add feature" not "Added feature").

## Reporting Issues

Use the GitHub issue templates for bug reports and feature requests. Include:

- Steps to reproduce (for bugs)
- Expected vs actual behavior
- Go version and OS

## License

By contributing, you agree that your contributions will be licensed under the MIT License.

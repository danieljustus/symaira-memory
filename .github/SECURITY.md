# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |

Only the latest release receives security updates.

## Reporting a Vulnerability

If you discover a security vulnerability in Symaira Memory, please report it responsibly.

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, please use [GitHub Private Vulnerability Reporting](https://github.com/danieljustus/symaira-memory/security/advisories/new) to report the issue privately.

Alternatively, you can email [daniel@symaira.dev](mailto:daniel@symaira.dev) with:

- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

## Response Timeline

- **Acknowledgment**: Within 48 hours
- **Initial assessment**: Within 1 week
- **Fix or mitigation**: Depends on severity, typically within 2 weeks

## Scope

This security policy applies to:

- The `symmemory` CLI binary
- The MCP server transport
- The HTTP REST API daemon
- The JWT authentication system
- The PII Guard redaction system
- Encrypted backup/restore functionality

## Out of Scope

- Security issues in dependencies (use Dependabot alerts)
- Issues requiring physical access to the user's machine
- Social engineering attacks

## Recognition

We appreciate the security research community and will acknowledge researchers who report valid vulnerabilities (with their permission).

# AI Agent Integration & MCP Guide

This guide describes how to connect the **Symaira Memory** Model Context Protocol (MCP) server and HTTP REST API daemon to popular AI host clients, IDE extensions, and web browsers.

---

## 🛠️ Recommended Setup

**Symaira Memory** (`symmemory`) is designed to operate local-first. AI agents use the protocol via stdio JSON-RPC 2.0 (for local CLI-based agents) or HTTP REST endpoints (for browser extension injections and other network clients).

To prevent agents from polluting the global database with ad-hoc project details, developers should:
1. Initialize local scoping with `.symmemory.toml` config files in project workspace roots (see [configuration.md](file:///Users/daniel/Dev/symaira-memory/docs/configuration.md)).
2. Configure agent profiles to set memories under the `project` or `session` scope rather than the fallback `global` scope.
3. Keep the API daemon secured using JSON Web Tokens (JWT) when listening on network ports.

---

## 🔌 Model Context Protocol (MCP) Integration

### Automatic Configuration
Running the following command displays a ready-to-use configuration block customized with your local binary's compiled execution paths:

```bash
symmemory mcp-config
```

---

## 📂 Claude Desktop Integration

Add the configuration block to your Claude Desktop configuration file:

*   **macOS Path**: `~/Library/Application Support/Claude/claude_desktop_config.json`
*   **Windows Path**: `%APPDATA%\Claude\claude_desktop_config.json`

### Configuration Block
```json
{
  "mcpServers": {
    "symaira-memory": {
      "command": "/usr/local/bin/symmemory",
      "args": ["serve"]
    }
  }
}
```

> [!IMPORTANT]
> Replace `/usr/local/bin/symmemory` with the absolute path to your compiled binary on your machine (e.g. `/Users/daniel/Dev/symaira-memory/symmemory`).

---

## 💻 Cursor Integration

To add the persistent memory server to Cursor:

1.  Navigate to **Cursor Settings** -> **Features** -> **MCP**.
2.  Click **+ Add New MCP Server**.
3.  Fill out the form:
    *   **Name**: `symaira-memory`
    *   **Type**: `stdio`
    *   **Command**: `/absolute/path/to/symmemory serve`
4.  Click **Save**. Cursor will connect immediately and display a green indicator next to the server name.

---

## 🔌 VS Code (Cline / Roo Code / Continue.dev)

Add the server to your VS Code MCP settings file (typically `~/.code/mcp_config.json` or within your active Cline extension settings):

```json
{
  "mcpServers": {
    "symaira-memory": {
      "command": "/absolute/path/to/symmemory",
      "args": ["serve"]
    }
  }
}
```

---

## 🛠️ Available MCP Tools

Once the MCP server is initialized, the following tools are registered with the host client:

| Tool Name | Arguments | Description |
| :--- | :--- | :--- |
| `memory_get` | `id` (string, required) | Retrieve a specific memory element from the SQLite database by its UUID. |
| `memory_set` | `content` (string, required)<br>`scope` (string, optional)<br>`metadata` (JSON string, optional) | Saves a new memory/fact. Runs offline pattern fact extraction, executes PII redactions, and automatically parses project directories. |
| `memory_search` | `query` (string, required)<br>`scope` (string, optional)<br>`profile` (string, optional)<br>`limit` (string, optional) | Semantic search of memories using cosine similarity over local vector embeddings. When `profile` is provided, searches across the scopes defined by that context profile in precedence order instead of a single `scope` filter. |
| `memory_list` | `scope` (string, optional) | Lists all stored memories, optionally filtering by scope level. |

---

## ⚡ HTTP API Daemon Mode (`symmemory serve -p 8787`)

For browser extensions, dashboard visualizers, or background scripts, Symaira Memory can run as an HTTP REST API daemon. Start the server on a specified port:

```bash
symmemory serve --port 8787
```

### Endpoints Reference
The daemon exposes standard REST API routes for database queries and updates. For the full OpenAPI specification, see [openapi.yaml](file:///Users/daniel/Dev/symaira-memory/docs/openapi.yaml).

*   **`GET /api/status`**
    *   *Purpose*: Health check.
    *   *Response*: `{"status":"healthy","version":"0.12.0","server":"symaira-memory"}`
*   **`POST /api/search`**
    *   *Purpose*: Semantic cosine-similarity search.
    *   *Payload*: `{"query": "database connection settings", "scope": "project", "limit": 3}`
*   **`POST /api/set`**
    *   *Purpose*: Ingest and persist a new fact.
    *   *Payload*: `{"content": "Daniel prefers TypeScript over Python", "scope": "global"}`
*   **`GET /api/list`**
    *   *Purpose*: Fetch all memories (supports `?scope=project` query parameter).

---

## 🔑 JWT Authentication for REST API

To secure your HTTP REST endpoints from unauthorized local process requests, Symaira Memory supports signed JSON Web Tokens (JWT) using a local HMAC-SHA256 signature scheme (see [jwt.go](file:///Users/daniel/Dev/symaira-memory/internal/security/jwt.go)).

### 1. Generating a Client Token
Use the `token generate` subcommand to issue client credentials:

```bash
symmemory token generate --subject "browser-extension" --duration 8760
```
This prints a cryptographically signed token valid for 8760 hours (1 year) specifically bounded to the `"browser-extension"` subject.

### 2. Verifying a Token
To check the signature, expiration, and payload details of a token:

```bash
symmemory token verify <token>
```

### 3. Client Request Usage
Pass the generated token in the HTTP request headers:

```http
Authorization: Bearer <your-generated-jwt-token>
```

---

## 🔄 Bidirectional Sync (`symmemory sync`)

Sync memories between a local instance and a remote Symaira Memory server. Uses last-write-wins (LWW) merge based on `updated_at` timestamps.

```bash
symmemory sync --remote http://remote-server:8787 --token <jwt-token>
```

The command performs:
1. **Pull**: Fetches changes from the remote server since the last sync cursor
2. **Apply**: Merges pulled records locally using LWW (newer `updated_at` wins)
3. **Push**: Sends local changes to the remote server
4. **Cursor update**: Saves the server timestamp as the new sync cursor

On first run (no cursor), all remote memories are pulled. Subsequent runs are incremental. Authentication failures (401) exit with a clear error message. The cursor is only advanced after both pull and push succeed.

---

## 🌐 Browser Extension Integration (Manifest V3)

The project includes a Google Chrome/Brave/Edge browser extension inside the [extension/](file:///Users/daniel/Dev/symaira-memory/extension) directory. It automatically integrates with ChatGPT, Claude Web, and Perplexity Web interfaces.

### Installation
1. Open Google Chrome and navigate to `chrome://extensions/`.
2. Enable **Developer Mode** using the toggle in the upper right.
3. Click **Load unpacked** and select the `/Users/daniel/Dev/symaira-memory/extension` directory.

### In-Page Context Injections
*   **Floating Button**: The content script ([content.js](file:///Users/daniel/Dev/symaira-memory/extension/content.js)) injects a premium floating button (`⚡`) inside the parent wrapper of conversational inputs.
*   **Context Lookup**: Clicking the button reads your draft input, generates search requests through the local daemon, and formats matching memories directly into a context block inside your text editor.
*   **Daemon Connection**: The extension connects to the API server running at `http://127.0.0.1:8787` via background service workers ([background.js](file:///Users/daniel/Dev/symaira-memory/extension/background.js)).

---

## 🔒 Security & Workspace Scoping

### PII Guard Regex Redaction
Before any memory content is committed to the local database, it is automatically passed through the security filter ([pii.go](file:///Users/daniel/Dev/symaira-memory/internal/security/pii.go)). This system automatically redacts:
*   Credit cards
*   Email addresses
*   General API tokens (`Authorization: Bearer ...` or `sk-proj-...` keys)

### Active Workspace Scoping
When a memory is recorded under `project` scope:
1. The tool traverses parent directories of the current working directory to detect a `.git` folder or a `.symmemory.toml` configuration file.
2. It parses the active project name and appends it to the memory metadata.
3. During agent queries, the agent can request memories matching the specific project scope, ensuring that context from "Project A" is not mixed with "Project B".

---

## 📋 Optimal Agent Instructions (System Prompt)

To ensure that connecting agents autonomously and correctly manage memory injection, search scoping, and fact formatting, see the copy-pasteable configuration template in [agent-instructions.md](file:///Users/daniel/Dev/symaira-memory/docs/agent-instructions.md).

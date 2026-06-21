# opencode MCP bridge

An HTTP bridge implemented in Go that allows MCP clients to connect to an existing OpenCode server via Streamable HTTP or a REST API. The bridge **will not** start OpenCode automatically.

## Quick Start

Requires Go 1.25+ and the `opencode` CLI. Start OpenCode first, then start the bridge:

```powershell
opencode serve --port 4096
$env:OPENCODE_BASE_URL = "http://127.0.0.1:4096"
go run ./cmd
```

The bridge listens on `:8080` by default. Available environment variables:

- `BRIDGE_LISTEN_ADDRESS`: Listen address, default `:8080`
- `BRIDGE_REQUEST_TIMEOUT`: Upstream HTTP timeout, default `60m`
- `OPENCODE_BASE_URL`: OpenCode server URL, default `http://127.0.0.1:4096`
- `OPENCODE_SERVER_USERNAME` / `OPENCODE_SERVER_PASSWORD`: HTTP Basic Auth credentials

## MCP Connection

The Streamable HTTP endpoint is `http://localhost:8080/mcp` (the root path `/` is also supported). Client configuration example:

```json
{
  "mcpServers": {
		"opencode": {
			"url": "http://localhost:8080/mcp/sse",
			"timeout": 3600
		}
  }
}
```

Legacy clients requiring the MCP 2024-11-05 HTTP+SSE transport can use `http://localhost:8080/mcp/sse`. Modern clients should prefer Streamable HTTP via `/mcp`.

The bridge exposes nine tools:

- `opencode_setup`
- `opencode_ask`
- `opencode_reply`
- `opencode_run`
- `opencode_check`
- `opencode_conversation`
- `opencode_sessions_overview`
- `opencode_mcp_servers`
- `opencode_provider_test`

## REST API

| Method | Path | Functionality |
|---|---|---|
| GET | `/opencode/setup` | Check service health and current project |
| POST | `/opencode/ask` | Create a new session and send a prompt |
| POST | `/opencode/reply` | Send a follow-up prompt to an existing session |
| POST | `/opencode/run` | Execute an asynchronous task and wait for completion |
| GET | `/opencode/check?sessionId=...` | Poll session status |
| GET | `/opencode/conversation?sessionId=...&limit=...` | Retrieve conversation history |
| GET | `/opencode/sessions-overview` | List all sessions |
| GET | `/opencode/mcp-servers` | List configured OpenCode MCP servers |
| POST | `/opencode/provider-test` | Test a specific provider/model |

REST call example:

```powershell
Invoke-RestMethod http://localhost:8080/opencode/ask `
  -Method Post -ContentType application/json `
  -Body '{"prompt":"Explain this project"}'
```

If OpenCode is unreachable, the API returns HTTP 503:

```json
{
  "error": "cannot connect to OpenCode server",
  "hint": "Please run `opencode serve` to start the OpenCode server first"
}
```

## Validation & Testing

```powershell
make check
make e2e
```

`make check` sequentially runs code formatting, vet, unit tests, and the build process. `make e2e` requires OpenCode to have an available provider/model configured; it starts OpenCode and the bridge, calls all nine MCP tools—including an ask/reply conversation chain and a cross-file audit task via `opencode_run`—and finally cleans up all test sessions and background processes.

`make test` generates `.run/coverage.out` and enforces a minimum overall statement coverage of 80% across core packages.

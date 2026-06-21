# Repository Guidelines

## Project Structure & Module Organization

This Go HTTP/MCP bridge uses `cmd/main.go` as its executable entry point. Keep responsibilities separated by package:

- `client/`: outbound OpenCode HTTP client and response parsing.
- `config/`: environment-based runtime configuration.
- `handlers/`: REST request validation and response handling.
- `mcpbridge/`: MCP server and tool definitions.
- `server/`: Echo routes and middleware.
- `types/`: shared request and response types.
- `scripts/`: PowerShell helpers for background services and end-to-end tests.

Tests live beside their implementation as `*_test.go`. Generated binaries and runtime logs belong in ignored `bin/` and `.run/` directories.

## Build, Test, and Development Commands

Use the root `Makefile` on Windows:

- `make deps`: download and normalize Go modules.
- `make run`: run the bridge from `./cmd`.
- `make build`: produce `bin/opencode-mcp-bridge.exe`.
- `make test`: run all unit tests.
- `make test-race`: run tests with the race detector.
- `make check`: format, vet, test, and build the project.
- `make e2e`: start OpenCode and the bridge, verify REST/MCP behavior, then clean up.

Use the `*-start` and `*-stop` targets for manual integration work.

## Coding Style & Naming Conventions

Follow standard Go conventions and run `gofmt`; `make fmt` formats all source packages. Use tabs as emitted by `gofmt`, short lowercase package names, PascalCase for exported identifiers, and camelCase for unexported identifiers. Keep HTTP transport logic in `handlers/` or `server/`, not in the client. PowerShell scripts should use approved verbs, explicit parameters, `$ErrorActionPreference = "Stop"`, and `finally` cleanup for background processes.

## Testing Guidelines

Use Go's `testing` and `net/http/httptest` packages. Name tests `TestBehavior`, for example `TestAskCreatesSessionAndSendsMessage`. Cover success, validation, upstream HTTP errors, and unavailable-service behavior. No numeric coverage threshold is defined; new behavior should include focused regression tests. Run `make check` before submitting, and `make e2e` when changing routes, MCP tools, startup scripts, or configuration.

## Commit & Pull Request Guidelines

The history contains only `Initial commit`, so no detailed convention is established. Use concise, imperative subjects such as `Add MCP tool discovery test`. Keep commits scoped to one coherent change. Pull requests should explain the behavior changed, list verification commands, link relevant issues, and document new environment variables or API changes. Include logs only when they clarify runtime behavior.

## Configuration & Security

Do not commit credentials, `.env`, `.run/` logs, or generated binaries. Configure endpoints and authentication through `OPENCODE_BASE_URL`, `OPENCODE_SERVER_USERNAME`, `OPENCODE_SERVER_PASSWORD`, `BRIDGE_LISTEN_ADDRESS`, and `BRIDGE_REQUEST_TIMEOUT`.

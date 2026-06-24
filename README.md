# greedy-eye-mcp

An [MCP](https://modelcontextprotocol.io) server that exposes the greedy-eye
backend to MCP clients (Claude and others) over **stdio**. The client launches
the binary directly and talks to it over stdin/stdout. Each tool is a thin, typed
proxy onto a backend **Connect-RPC** call.

## Quickstart

```bash
# 1. Resolve deps and build
make bootstrap            # -> bin/server

# 2. Run it manually (normally the MCP client launches it for you)
GREEDY_EYE_BACKEND_URL=http://localhost:8080 ./bin/server
```

This module depends on the backend Go module `github.com/foxcool/greedy-eye`
for the API contract, resolved via a local `replace => ../greedy-eye` in
`go.mod`. Keep the sibling checkout present.

The process speaks JSON-RPC over stdout; all logs go to stderr.

## Connecting a client

Point claude desktop/code at the built (or released) binary:

```json
{
  "mcpServers": {
    "greedy-eye": {
      "command": "/absolute/path/to/greedy-eye-mcp",
      "env": {
        "GREEDY_EYE_BACKEND_URL": "https://your-eye-host",
        "GREEDY_EYE_AUTH_TOKEN": "psn_..."
      }
    }
  }
}
```

Prebuilt binaries per OS/arch are published as archives via
[GoReleaser](https://goreleaser.com) (`make release`); `make snapshot` builds
them locally into `dist/`.

## Configuration

All configuration is via environment variables:

| Variable                  | Default                  | Description                                            |
| ------------------------- | ------------------------ | ------------------------------------------------------ |
| `GREEDY_EYE_BACKEND_URL`  | `http://localhost:8080`  | Base URL of the greedy-eye Connect-RPC backend.        |
| `GREEDY_EYE_AUTH_TOKEN`   | _(empty)_                | psina personal access token, sent as `Authorization: Bearer`. Required behind psina ForwardAuth; empty for direct-to-eye dev. |
| `BACKEND_PROTOCOL`        | `connect`                | `connect` or `grpc`.                                   |
| `REQUEST_TIMEOUT`         | `30s`                    | Per-call timeout to the backend.                       |
| `ENABLE_MUTATIONS`        | `false`                  | Gate for write/execute tools (none implemented yet).   |

### Minting an auth token

The backend sits behind Traefik with a psina ForwardAuth middleware. Mint a
long-lived **personal access token** and put the returned `psn_...` secret in
`GREEDY_EYE_AUTH_TOKEN`. Two ways:

- **Frontend** — the `/settings` page (Access tokens) creates, lists, and revokes
  tokens; the secret is shown once.
- **API** — `auth.v1.AuthService/CreatePersonalAccessToken` (authenticated with a
  normal access token); revoke via `RevokePersonalAccessToken`.

## Tools

Read-only by default. Names are namespaced with `eye_`.

- `eye_list_assets`, `eye_get_asset`
- `eye_get_latest_price` (adds a human-readable price), `eye_list_price_history`
- `eye_list_portfolios`, `eye_get_portfolio`, `eye_list_holdings`
- `eye_calculate_portfolio_value` (adds a human-readable total)
- `eye_list_rules`, `eye_get_rule`, `eye_simulate_rule` (dry-run only)

Money-moving operations (`ExecuteRule`, create/delete) are intentionally not
exposed. See `AGENTS.md`.

## Why a proxy, and why Go

The backend already owns all state and logic. This server only translates MCP
tool calls into Connect-RPC calls, so it is stateless per request. Go fits: a
single static binary, fast startup, goroutines for concurrent outbound calls,
and first-class gRPC/Connect tooling — no runtime to ship.

## Decimals

greedy-eye stores balances and prices as raw integers scaled by a `decimals`
field (uint256 on-chain values overflow int64). Tools return the raw value and,
where useful, a `*_human` field with the decimal point applied.

## Distribution

`make release` runs GoReleaser to build per-OS/arch archives
(`linux`/`darwin` × `amd64`/`arm64`) and publish them to GitHub Releases. No
container image is shipped — the server runs as a local stdio binary launched by
the client. `make snapshot` produces the same archives locally without
publishing.

CI-driven, tag-triggered releases are blocked until the backend `api/v1` package
is published in a tag (see the `replace` caveat in `AGENTS.md`); for now, run
GoReleaser locally where the sibling checkout is present.

## API contract

No protos or codegen live here. The contract is imported directly from the
backend Go module (`github.com/foxcool/greedy-eye/api/v1` and its
`apiv1connect` subpackage). See `AGENTS.md` for the dependency setup.

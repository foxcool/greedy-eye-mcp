# greedy-eye-mcp

An [MCP](https://modelcontextprotocol.io) server that exposes the greedy-eye
backend to MCP clients (Claude and others) over **Streamable HTTP**. Each tool is
a thin, typed proxy onto a backend **Connect-RPC** call.

## Quickstart

```bash
# 1. Resolve deps and build
make bootstrap

# 2. Point it at a running greedy-eye backend and run
GREEDY_EYE_BACKEND_URL=http://localhost:8080 make run
```

This module depends on the backend Go module `github.com/foxcool/greedy-eye`
for the API contract, resolved via a local `replace => ../greedy-eye` in
`go.mod`. Keep the sibling checkout present.

The MCP endpoint is then served at `http://localhost:8090/mcp`, with a
`/healthz` probe alongside it.

## Configuration

All configuration is via environment variables:

| Variable                  | Default                  | Description                                            |
| ------------------------- | ------------------------ | ------------------------------------------------------ |
| `GREEDY_EYE_BACKEND_URL`  | `http://localhost:8080`  | Base URL of the greedy-eye Connect-RPC backend.        |
| `MCP_LISTEN_ADDR`         | `:8090`                  | Address the MCP HTTP server binds to.                  |
| `BACKEND_PROTOCOL`        | `connect`                | `connect` or `grpc`.                                   |
| `REQUEST_TIMEOUT`         | `30s`                    | Per-call timeout to the backend.                       |
| `ENABLE_MUTATIONS`        | `false`                  | Gate for write/execute tools (none implemented yet).   |

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

## Deployment

Build the image:

```bash
make docker
```

Deploy as a normal long-running service (e.g. a Kubernetes Deployment + Service).
To use it as a remote connector in a client, expose it via a public ingress: the
connection originates from the client vendor's servers, not your machine, so a
cluster-internal address will not be reachable. For local-only use, front it with
a stdio bridge.

## API contract

No protos or codegen live here. The contract is imported directly from the
backend Go module (`github.com/foxcool/greedy-eye/api/v1` and its
`apiv1connect` subpackage). See `AGENTS.md` for the dependency setup and the
Dockerfile build-context caveat.

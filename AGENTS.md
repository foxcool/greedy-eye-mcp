# Agent Instructions — greedy-eye-mcp

MCP server that exposes the greedy-eye backend to MCP clients (Claude and others)
over Streamable HTTP. Each tool is a thin, typed proxy onto a backend
Connect-RPC call.

## Architecture

- Transport to clients: **Streamable HTTP** (mark3labs/mcp-go), mounted at `/mcp`.
- Transport to backend: **Connect-RPC** (`connectrpc.com/connect`). The greedy-eye
  backend speaks Connect/gRPC/gRPC-Web; we default to the Connect protocol over
  HTTP/1.1. `BACKEND_PROTOCOL=grpc` switches to gRPC (HTTP/2; h2c for plaintext).
- This is a pure proxy: tool call -> Connect-RPC call -> response. It holds no
  state of its own. BEAM-style concurrency advantages do not apply here; Go's
  goroutines + a connection pool are the right fit.

## Protobuf contract

There are no protos or codegen in this repo. The API contract comes from the
backend Go module: we import `github.com/foxcool/greedy-eye/api/v1` (messages)
and `.../api/v1/apiv1connect` (clients) directly. The backend owns the protos
and the generated code; we depend on it.

`go.mod` pins the backend with a local `replace` directive
(`=> ../greedy-eye`) so it resolves offline against the sibling checkout, with
no tags or GOPRIVATE needed. See the build-context TODO in the Dockerfile: the
`replace` points outside the Docker build context and must be addressed before
containerizing (move context to `ge/`, or tag+push the backend and drop the
replace).

## First-run setup

```bash
make bootstrap   # tidy + build
```

## Layout

- `internal/config/` — env-based configuration.
- `internal/backend/` — Connect-RPC client construction (typed clients from the
  backend `apiv1connect` package).
- `internal/mcpserver/` — MCP server, tool registration, formatting helpers.
- `cmd/server/` — entrypoint (HTTP server, graceful shutdown).

## Safety: mutating tools are gated

Only read-only / non-mutating tools are registered by default. Create, delete,
and especially `ExecuteRule` (which can trigger real trades and withdrawals) are
NOT exposed. They are gated behind `ENABLE_MUTATIONS=true` and a deliberate
`registerMutatingTools` implementation that does not yet exist. Do not expose
money-moving operations to an LLM surface without explicit confirmation
semantics.

## Deployment note (remote MCP)

When added to a client as a remote connector, the connection originates from the
client vendor's servers, not from the local machine. A server deployed inside a
private cluster is unreachable unless exposed via a public ingress. For
local-only use, front it with a stdio bridge instead.

## Conventions

- Code, comments, and commit messages in English.
- Vendor-specific assistant configs stay in `.gitignore`, never committed.
- Ship working over polished; iterate.

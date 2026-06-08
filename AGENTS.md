# Agent Instructions — greedy-eye-mcp

MCP server that exposes the greedy-eye backend to MCP clients (Claude and others)
over stdio. Each tool is a thin, typed proxy onto a backend Connect-RPC call.

## Architecture

- Transport to clients: **stdio** (mark3labs/mcp-go `ServeStdio`). The client
  launches the binary; JSON-RPC flows over stdin/stdout. **stdout is the
  protocol — all logging must go to stderr.**
- Transport to backend: **Connect-RPC** (`connectrpc.com/connect`). The greedy-eye
  backend speaks Connect/gRPC/gRPC-Web; we default to the Connect protocol over
  HTTP/1.1. `BACKEND_PROTOCOL=grpc` switches to gRPC (HTTP/2; h2c for plaintext).
- This is a pure proxy: tool call -> Connect-RPC call -> response. It holds no
  state of its own. BEAM-style concurrency advantages do not apply here; Go's
  goroutines + a connection pool are the right fit.
- Auth: when `GREEDY_EYE_AUTH_TOKEN` is set, a Connect interceptor
  (`internal/backend/client.go authInterceptor`) attaches `Authorization: Bearer
  <token>` to every backend call. The backend is fronted by psina ForwardAuth,
  which verifies the token (an opaque psina personal access token, `psn_...`) and
  injects `X-User-Id`. We never send `X-User-Id` ourselves — Traefik overwrites
  it from psina's response.

## Protobuf contract

There are no protos or codegen in this repo. The API contract comes from the
backend Go module: we import `github.com/foxcool/greedy-eye/api/v1` (messages)
and `.../api/v1/apiv1connect` (clients) directly. The backend owns the protos
and the generated code; we depend on it.

`go.mod` pins the backend with a local `replace` directive
(`=> ../greedy-eye`) so it resolves offline against the sibling checkout, with
no tags or GOPRIVATE needed. This works for local builds and local GoReleaser
runs. CI releases are blocked by it: CI checks out only this repo, so the sibling
is absent. To unblock CI, tag the backend with a version containing `api/v1`,
bump the `require`, and drop the `replace` line.

## First-run setup

```bash
make bootstrap   # tidy + build
```

## Layout

- `internal/config/` — env-based configuration.
- `internal/backend/` — Connect-RPC client construction (typed clients from the
  backend `apiv1connect` package).
- `internal/mcpserver/` — MCP server, tool registration, formatting helpers.
- `cmd/server/` — entrypoint (stdio serve loop, logs to stderr).

## Safety: mutating tools are gated

Only read-only / non-mutating tools are registered by default. Create, delete,
and especially `ExecuteRule` (which can trigger real trades and withdrawals) are
NOT exposed. They are gated behind `ENABLE_MUTATIONS=true` and a deliberate
`registerMutatingTools` implementation that does not yet exist. Do not expose
money-moving operations to an LLM surface without explicit confirmation
semantics.

## Distribution

GoReleaser builds per-OS/arch archives (no Docker image); the binary runs locally
as a stdio server launched by the client. `make snapshot` builds locally,
`make release` publishes to GitHub Releases. Run it locally for now — see the
`replace` caveat above for why CI releases are deferred.

## Conventions

- Code, comments, and commit messages in English.
- Vendor-specific assistant configs stay in `.gitignore`, never committed.
- Ship working over polished; iterate.

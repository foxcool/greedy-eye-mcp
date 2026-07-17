// Package mcpserver wires the greedy-eye backend into an MCP server that Claude
// (or any MCP client) can connect to over stdio. Each tool is a thin, typed
// proxy onto a backend Connect-RPC call.
package mcpserver

import (
	"github.com/mark3labs/mcp-go/server"

	"github.com/foxcool/greedy-eye-mcp/internal/backend"
	"github.com/foxcool/greedy-eye-mcp/internal/config"
)

const (
	serverName    = "greedy-eye-mcp"
	serverVersion = "0.1.0"
)

// serverInstructions is returned to the client on initialize. Keep it short:
// the full import workflow lives in docs/importing.md.
const serverInstructions = `greedy-eye portfolio tools. Money amounts are raw integers scaled by a
'decimals' field unless a *_human field is present. Write tools (when enabled)
follow a simulation-first contract: imports default to dry_run=true and return
a per-item plan — show the plan to the user and get explicit confirmation
before repeating the call with dry_run=false. Never invent amounts or symbols:
ask the user when an export is ambiguous.`

// New builds an MCP server and registers the tool set. The caller drives it over
// a transport (stdio).
func New(cfg config.Config, clients *backend.Clients) *server.MCPServer {
	s := server.NewMCPServer(
		serverName,
		serverVersion,
		server.WithToolCapabilities(true),
		server.WithInstructions(serverInstructions),
	)

	// Read-only tools are always registered.
	registerMarketDataTools(s, clients)
	registerPortfolioTools(s, clients)
	registerAutomationTools(s, clients)
	registerAnalyticsTools(s, clients)

	// Mutating tools are opt-in: they write accounts, assets, holdings, and
	// transaction history. Import tools default to dry_run=true; committing
	// requires an explicit dry_run=false after the plan is confirmed.
	if cfg.EnableMutations {
		registerMutatingTools(s, clients)
	}

	return s
}

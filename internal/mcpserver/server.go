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

// New builds an MCP server and registers the tool set. The caller drives it over
// a transport (stdio).
func New(cfg config.Config, clients *backend.Clients) *server.MCPServer {
	s := server.NewMCPServer(
		serverName,
		serverVersion,
		server.WithToolCapabilities(true),
	)

	// Read-only tools are always registered.
	registerMarketDataTools(s, clients)
	registerPortfolioTools(s, clients)
	registerAutomationTools(s, clients)

	// Mutating tools (create/delete/execute) are opt-in. ExecuteRule can trigger
	// real trades and withdrawals, so they stay gated behind cfg.EnableMutations.
	// TODO: add deliberately, with care:
	//   if cfg.EnableMutations {
	//       registerMutatingTools(s, clients)
	//   }

	return s
}

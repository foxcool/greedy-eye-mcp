// Package mcpserver wires the greedy-eye backend into an MCP server that Claude
// (or any MCP client) can connect to over Streamable HTTP. Each tool is a thin,
// typed proxy onto a backend Connect-RPC call.
package mcpserver

import (
	"net/http"

	"github.com/mark3labs/mcp-go/server"

	"github.com/foxcool/greedy-eye-mcp/internal/backend"
	"github.com/foxcool/greedy-eye-mcp/internal/config"
)

const (
	serverName    = "greedy-eye-mcp"
	serverVersion = "0.1.0"
	mcpPath       = "/mcp"
)

// New builds an MCP server, registers the tool set, and returns an http.Handler
// that exposes the MCP endpoint at /mcp plus a /healthz probe for Kubernetes.
func New(cfg config.Config, clients *backend.Clients) http.Handler {
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
	// real trades and withdrawals, so it stays gated behind explicit config.
	if cfg.EnableMutations {
		// registerMutatingTools(s, clients) // TODO: add deliberately, with care.
	}

	mux := http.NewServeMux()
	mux.Handle(mcpPath, server.NewStreamableHTTPServer(s))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

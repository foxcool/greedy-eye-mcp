// Command server runs the greedy-eye MCP server: a stdio MCP endpoint that
// proxies tool calls onto the greedy-eye Connect-RPC backend. It is launched
// directly by an MCP client (claude desktop/code) and speaks JSON-RPC over
// stdin/stdout, so all logging goes to stderr.
package main

import (
	"log"
	"log/slog"
	"os"

	"github.com/mark3labs/mcp-go/server"

	"github.com/foxcool/greedy-eye-mcp/internal/backend"
	"github.com/foxcool/greedy-eye-mcp/internal/config"
	"github.com/foxcool/greedy-eye-mcp/internal/mcpserver"
)

func main() {
	// stdout carries the MCP protocol; logs must not touch it.
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	clients := backend.New(cfg)
	s := mcpserver.New(cfg, clients)

	logger.Info("starting MCP stdio server",
		"backend", cfg.BackendURL,
		"protocol", cfg.Protocol,
		"mutations_enabled", cfg.EnableMutations,
	)

	// ServeStdio reads from stdin / writes to stdout and handles SIGINT/SIGTERM
	// for graceful shutdown. Its internal error logger is routed to stderr.
	errLog := log.New(os.Stderr, "", log.LstdFlags)
	if err := server.ServeStdio(s, server.WithErrorLogger(errLog)); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
	logger.Info("server stopped")
}

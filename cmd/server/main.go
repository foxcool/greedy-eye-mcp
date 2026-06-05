// Command server runs the greedy-eye MCP server: a Streamable HTTP MCP endpoint
// that proxies tool calls onto the greedy-eye Connect-RPC backend.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/foxcool/greedy-eye-mcp/internal/backend"
	"github.com/foxcool/greedy-eye-mcp/internal/config"
	"github.com/foxcool/greedy-eye-mcp/internal/mcpserver"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	clients := backend.New(cfg)
	handler := mcpserver.New(cfg, clients)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Run the HTTP server in the background.
	errCh := make(chan error, 1)
	go func() {
		logger.Info("starting MCP server",
			"addr", cfg.ListenAddr,
			"backend", cfg.BackendURL,
			"protocol", cfg.Protocol,
			"mutations_enabled", cfg.EnableMutations,
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// Wait for a termination signal or a fatal server error.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		logger.Error("server failed", "error", err)
		os.Exit(1)
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("server stopped")
}

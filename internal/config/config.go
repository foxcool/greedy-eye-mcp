// Package config loads runtime configuration from the environment.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds all runtime settings for the MCP server.
type Config struct {
	// BackendURL is the base URL of the greedy-eye Connect-RPC backend
	// (for example "http://localhost:8080" or "http://greedy-eye:8080").
	BackendURL string
	// AuthToken is an optional psina credential (a personal access token) sent as
	// "Authorization: Bearer <token>" on every backend call. Required when the
	// backend sits behind psina ForwardAuth; empty for direct-to-eye dev.
	AuthToken string
	// Protocol selects how we talk to the backend: "connect" (default) or "grpc".
	// Connect works over plain HTTP/1.1; grpc requires HTTP/2 (h2c for plaintext).
	Protocol string
	// RequestTimeout bounds a single backend call.
	RequestTimeout time.Duration
	// EnableMutations gates write/execute tools (create, delete, execute-rule, ...).
	// Off by default: an LLM-facing surface should not be able to move money
	// or mutate state unless explicitly opted in.
	EnableMutations bool
}

// Load reads configuration from environment variables, applying defaults.
func Load() (Config, error) {
	cfg := Config{
		BackendURL:      getEnv("GREEDY_EYE_BACKEND_URL", "http://localhost:8080"),
		AuthToken:       getEnv("GREEDY_EYE_AUTH_TOKEN", ""),
		Protocol:        strings.ToLower(getEnv("BACKEND_PROTOCOL", "connect")),
		EnableMutations: getEnv("ENABLE_MUTATIONS", "false") == "true",
	}

	timeoutStr := getEnv("REQUEST_TIMEOUT", "30s")
	d, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return Config{}, fmt.Errorf("invalid REQUEST_TIMEOUT %q: %w", timeoutStr, err)
	}
	cfg.RequestTimeout = d

	switch cfg.Protocol {
	case "connect", "grpc":
	default:
		return Config{}, fmt.Errorf("invalid BACKEND_PROTOCOL %q (want connect|grpc)", cfg.Protocol)
	}

	if cfg.BackendURL == "" {
		return Config{}, fmt.Errorf("GREEDY_EYE_BACKEND_URL must not be empty")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

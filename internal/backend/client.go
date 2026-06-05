// Package backend constructs typed Connect-RPC clients for the greedy-eye backend.
package backend

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"

	"github.com/foxcool/greedy-eye-mcp/internal/config"
	apiv1connect "github.com/foxcool/greedy-eye/api/v1/apiv1connect"
)

// Clients bundles one typed client per backend service.
type Clients struct {
	MarketData apiv1connect.MarketDataServiceClient
	Portfolio  apiv1connect.PortfolioServiceClient
	Automation apiv1connect.AutomationServiceClient
}

// New builds the backend clients according to the configured protocol.
func New(cfg config.Config) *Clients {
	httpClient, opts := transport(cfg)
	base := strings.TrimRight(cfg.BackendURL, "/")

	return &Clients{
		MarketData: apiv1connect.NewMarketDataServiceClient(httpClient, base, opts...),
		Portfolio:  apiv1connect.NewPortfolioServiceClient(httpClient, base, opts...),
		Automation: apiv1connect.NewAutomationServiceClient(httpClient, base, opts...),
	}
}

// transport returns an HTTP client and connect options suited to the protocol.
//
//   - "connect" (default): plain http.Client, Connect protocol over HTTP/1.1.
//   - "grpc" over https://: standard http.Client negotiates HTTP/2 via ALPN.
//   - "grpc" over http://:  needs an explicit h2c (cleartext HTTP/2) transport.
func transport(cfg config.Config) (connect.HTTPClient, []connect.ClientOption) {
	if cfg.Protocol == "grpc" {
		opts := []connect.ClientOption{connect.WithGRPC()}
		if strings.HasPrefix(cfg.BackendURL, "http://") {
			return h2cClient(), opts
		}
		return &http.Client{Timeout: cfg.RequestTimeout}, opts
	}
	return &http.Client{Timeout: cfg.RequestTimeout}, nil
}

// h2cClient dials cleartext HTTP/2 (h2c) for plaintext gRPC backends.
func h2cClient() *http.Client {
	return &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, addr)
			},
		},
	}
}

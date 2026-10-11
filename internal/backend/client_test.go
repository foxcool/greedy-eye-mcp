package backend

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A plaintext gRPC backend speaks HTTP/2 without TLS; the client must not fall
// back to HTTP/1.1, which a gRPC server refuses.
func TestH2CClientSpeaksCleartextHTTP2(t *testing.T) {
	var proto int
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proto = r.ProtoMajor
	}))
	srv.Config.Protocols = new(http.Protocols)
	srv.Config.Protocols.SetUnencryptedHTTP2(true)
	srv.Start()
	defer srv.Close()

	resp, err := h2cClient().Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if proto != 2 {
		t.Fatalf("request arrived over HTTP/%d, want cleartext HTTP/2", proto)
	}
}

package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/foxcool/greedy-eye-mcp/internal/backend"
	"github.com/foxcool/greedy-eye-mcp/internal/buildinfo"
	"github.com/foxcool/greedy-eye-mcp/internal/config"
)

func rpc(t *testing.T, method, params string) string {
	t.Helper()
	s := New(config.Config{}, &backend.Clients{})
	msg := `{"jsonrpc":"2.0","id":1,"method":"` + method + `","params":` + params + `}`
	resp := s.HandleMessage(context.Background(), json.RawMessage(msg))
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInitializeReportsBuild(t *testing.T) {
	out := rpc(t, "initialize", `{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"t","version":"0"}}`)
	want, _ := json.Marshal(buildinfo.String())
	if !strings.Contains(out, `"version":`+string(want)) {
		t.Errorf("serverInfo.version is not the build string %s in %s", want, out)
	}
}

func TestVersionToolReportsBuild(t *testing.T) {
	out := rpc(t, "tools/call", `{"name":"eye_get_server_version","arguments":{}}`)
	want, _ := json.Marshal(buildinfo.String())
	if !strings.Contains(out, strings.Trim(string(want), `"`)) || strings.Contains(out, `"isError":true`) {
		t.Errorf("version tool did not return the build string: %s", out)
	}
}

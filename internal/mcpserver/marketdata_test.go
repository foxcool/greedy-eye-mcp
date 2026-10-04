package mcpserver

import (
	"slices"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func callWith(args map[string]any) mcp.CallToolRequest {
	var req mcp.CallToolRequest
	req.Params.Arguments = args
	return req
}

// A search used to mean paging the whole catalogue and filtering by eye; the
// backend answers it directly since v0.19.1, and the tool has to pass it on.
func TestListAssetsRequestCarriesSearch(t *testing.T) {
	in := listAssetsRequest(callWith(map[string]any{
		"query": "usdt",
		"ids":   []any{"a1", "a2"},
	}))
	if in.Query == nil || *in.Query != "usdt" {
		t.Errorf("query not passed on: %v", in.Query)
	}
	if !slices.Equal(in.Ids, []string{"a1", "a2"}) {
		t.Errorf("ids not passed on: %v", in.Ids)
	}
}

// Absent stays absent: an empty query is no query, and no ids is no id filter.
func TestListAssetsRequestOmitsWhatWasNotAsked(t *testing.T) {
	in := listAssetsRequest(callWith(map[string]any{"query": ""}))
	if in.Query != nil {
		t.Errorf("an empty query must not be sent, got %q", *in.Query)
	}
	if len(in.Ids) != 0 {
		t.Errorf("no ids must mean no id filter, got %v", in.Ids)
	}
}

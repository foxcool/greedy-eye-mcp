package mcpserver

import (
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	apiv1 "github.com/foxcool/greedy-eye/api/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestResetPlanEntryCountsUnaskable(t *testing.T) {
	e := resetPlanEntry("moex", &apiv1.SourceSchedule{Deferred: 3, Unaskable: 4})
	if got := e["would_free"]; got != uint32(7) {
		t.Errorf("would_free = %v, want deferred+unaskable = 7", got)
	}
	if got := e["unaskable"]; got != uint32(4) {
		t.Errorf("unaskable = %v, want 4", got)
	}
}

func TestResetPlanEntryOmitsAbsentLatestDeferred(t *testing.T) {
	if _, ok := resetPlanEntry("moex", &apiv1.SourceSchedule{})["latest_deferred"]; ok {
		t.Error("latest_deferred must be omitted when absent, not printed as the epoch")
	}
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	e := resetPlanEntry("moex", &apiv1.SourceSchedule{LatestDeferred: timestamppb.New(ts)})
	if e["latest_deferred"] != "2026-01-02T03:04:05Z" {
		t.Errorf("latest_deferred = %v", e["latest_deferred"])
	}
}

func TestFindOrCreateAssetDefaultsToDryRun(t *testing.T) {
	call := func(args map[string]any) *apiv1.FindOrCreateAssetRequest {
		t.Helper()
		var req mcp.CallToolRequest
		req.Params.Arguments = args
		in, err := findOrCreateAssetRequest(req)
		if err != nil {
			t.Fatal(err)
		}
		return in
	}
	if !call(map[string]any{"symbol": "BTC"}).GetDryRun() {
		t.Error("dry_run omitted: want true, got false")
	}
	if call(map[string]any{"symbol": "BTC", "dry_run": false}).GetDryRun() {
		t.Error("explicit dry_run=false must commit")
	}
}

func TestMergeAssetsDefaultsToDryRun(t *testing.T) {
	args := func(extra map[string]any) map[string]any {
		m := map[string]any{
			"source_id": "s", "target_id": "t",
			"expected_source_symbol": "USDT", "expected_target_symbol": "USDT",
		}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	call := func(a map[string]any) (*apiv1.MergeAssetsRequest, error) {
		var req mcp.CallToolRequest
		req.Params.Arguments = a
		return mergeAssetsRequest(req)
	}

	in, err := call(args(nil))
	if err != nil {
		t.Fatal(err)
	}
	if in.DryRun == nil || !*in.DryRun {
		t.Error("dry_run omitted: want an explicit true")
	}
	if in, _ := call(args(map[string]any{"dry_run": false})); in.GetDryRun() {
		t.Error("explicit dry_run=false must commit")
	}
	missing := args(nil)
	delete(missing, "expected_target_symbol")
	if _, err := call(missing); err == nil {
		t.Error("a missing expected symbol must be refused")
	}
}

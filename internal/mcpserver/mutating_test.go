package mcpserver

import (
	"testing"
	"time"

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

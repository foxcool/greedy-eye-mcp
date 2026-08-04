package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"

	apiv1 "github.com/foxcool/greedy-eye/api/v1"
)

// TestCoverageNote_NamesWhatIsMissing: the counts are in the response already,
// but a number in a nested field is easy to summarise past. The sentence has to
// carry the number that is missing and the instruction to qualify the total.
func TestCoverageNote_NamesWhatIsMissing(t *testing.T) {
	note := coverageNote(&apiv1.ValuationCoverage{
		PricedCount:   108,
		UnpricedCount: 131,
		Unpriced: []*apiv1.UnpricedHolding{
			{Symbol: "AMB", Reason: apiv1.UnpricedReason_UNPRICED_REASON_NO_QUOTE},
			{Symbol: "RADAR", Reason: apiv1.UnpricedReason_UNPRICED_REASON_THIN_MARKET},
			{Symbol: "IDEX", Reason: apiv1.UnpricedReason_UNPRICED_REASON_THIN_MARKET},
		},
		UnpricedTruncated: true,
	})

	for _, want := range []string{"108 of 239", "131 holding(s) are NOT included", "capped sample", "priced holdings only"} {
		if !strings.Contains(note, want) {
			t.Errorf("coverage note must contain %q, got: %s", want, note)
		}
	}
	if !strings.Contains(note, "1 of the 3 listed have no quote") || !strings.Contains(note, "2 have a quote with no market") {
		t.Errorf("reasons must be counted over the disclosed sample, got: %s", note)
	}
}

// TestCoverageNote_FullCoverageStillSpeaks: a complete valuation says so. Silence
// would be indistinguishable from a tool that never checked.
func TestCoverageNote_FullCoverageStillSpeaks(t *testing.T) {
	note := coverageNote(&apiv1.ValuationCoverage{PricedCount: 12})
	if !strings.Contains(note, "all 12 holdings were priced") {
		t.Errorf("unexpected note: %s", note)
	}

	empty := coverageNote(&apiv1.ValuationCoverage{})
	if !strings.Contains(empty, "values nothing") {
		t.Errorf("an empty scope must not read as a complete valuation: %s", empty)
	}

	if got := coverageNote(nil); got != "" {
		t.Errorf("a response without coverage says nothing rather than inventing it, got: %s", got)
	}
}

// TestResultProtoWith_AddsFieldsAndDropsEmptyOnes: enrichment lands at the top
// level next to the proto fields, and an empty derived value is omitted rather
// than shown as an empty string a model might read as "none".
func TestResultProtoWith_AddsFieldsAndDropsEmptyOnes(t *testing.T) {
	res, err := resultProtoWith(&apiv1.PortfolioValueResponse{
		PortfolioId:      "p1",
		TotalValueAmount: "756870",
		Decimals:         2,
	}, map[string]any{
		"total_value_human": "7568.70",
		"coverage_note":     "",
	})
	if err != nil {
		t.Fatalf("resultProtoWith: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(textOf(t, res)), &m); err != nil {
		t.Fatalf("tool result is not JSON: %v", err)
	}
	if m["total_value_human"] != "7568.70" {
		t.Errorf("derived field missing: %v", m)
	}
	if _, ok := m["coverage_note"]; ok {
		t.Errorf("an empty derived field must be omitted: %v", m)
	}
	if m["portfolio_id"] != "p1" {
		t.Errorf("proto fields must survive enrichment: %v", m)
	}
}

// textOf extracts the single text payload of a tool result.
func textOf(t *testing.T, res any) string {
	t.Helper()
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var envelope struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(b, &envelope); err != nil {
		t.Fatalf("unmarshal result envelope: %v", err)
	}
	if len(envelope.Content) != 1 {
		t.Fatalf("expected one content item, got %d", len(envelope.Content))
	}
	return envelope.Content[0].Text
}

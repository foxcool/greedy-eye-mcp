package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	apiv1 "github.com/foxcool/greedy-eye/api/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
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

	for _, want := range []string{"108 of 239", "131 holding(s) are OUT of the total", "capped sample", "priced holdings only"} {
		if !strings.Contains(note, want) {
			t.Errorf("coverage note must contain %q, got: %s", want, note)
		}
	}
	if !strings.Contains(note, "of the 3 listed, 1 have no quote yet") || !strings.Contains(note, "2 have a quote with no market") {
		t.Errorf("reasons must be counted over the disclosed sample, got: %s", note)
	}
}

// TestCoverageNote_NeverPricedIsItsOwnClaim: production reported nine ETFs as
// "no quote at all" while asked_since sat in the field beside them — the note
// folded NEVER_PRICED into the default bucket. The two say different things
// about where the gap is, and the sentence a model quotes must keep them apart.
func TestCoverageNote_NeverPricedIsItsOwnClaim(t *testing.T) {
	note := coverageNote(&apiv1.ValuationCoverage{
		UnpricedCount: 2,
		Unpriced: []*apiv1.UnpricedHolding{
			{Symbol: "FXGD", Reason: apiv1.UnpricedReason_UNPRICED_REASON_NEVER_PRICED},
			{Symbol: "FXRU", Reason: apiv1.UnpricedReason_UNPRICED_REASON_NEVER_PRICED},
		},
	})

	if !strings.Contains(note, "2 have been asked of every source available and never answered") {
		t.Errorf("NEVER_PRICED must be stated as its own reason, got: %s", note)
	}
	if strings.Contains(note, "no quote yet") {
		t.Errorf("NEVER_PRICED must not be reported as an unasked asset, got: %s", note)
	}
	if !strings.Contains(note, "not a delisting verdict") {
		t.Errorf("silence from sources is evidence, not a verdict; the note must say so: %s", note)
	}
}

// TestCoverageNote_StaleIsInTheTotal: stale holdings are IN the total and
// unpriced ones are OUT of it. A reader who reads both as "missing" subtracts
// the same doubt twice, so the wording carries the direction explicitly.
func TestCoverageNote_StaleIsInTheTotal(t *testing.T) {
	note := coverageNote(&apiv1.ValuationCoverage{PricedCount: 40, StaleCount: 3})

	if !strings.Contains(note, "3 priced holding(s) are IN the total") {
		t.Errorf("stale holdings must be reported as included, got: %s", note)
	}
	if !strings.Contains(note, "do not subtract them a second time") {
		t.Errorf("the note must block double subtraction, got: %s", note)
	}
}

// TestCoverageNote_SilentWhenNothingIsStale: a "0 stale" clause trains the
// reader to skip the sentence on the day the number is not zero.
func TestCoverageNote_SilentWhenNothingIsStale(t *testing.T) {
	note := coverageNote(&apiv1.ValuationCoverage{PricedCount: 40})

	if strings.Contains(note, "stale") || strings.Contains(note, "IN the total") {
		t.Errorf("staleness must go unmentioned when nothing is stale, got: %s", note)
	}
}

// TestCoverageNote_DatesBothAxes: a price and an amount go stale independently
// and only the price has a sweep watching it. Naming one and omitting the other
// invites the reader to assume the omitted one is current.
func TestCoverageNote_DatesBothAxes(t *testing.T) {
	fixed := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	now = func() time.Time { return fixed }
	t.Cleanup(func() { now = time.Now })

	note := coverageNote(&apiv1.ValuationCoverage{
		PricedCount: 85,
		PricesAsOf:  timestamppb.New(fixed.Add(-6 * time.Hour)),
		AmountsAsOf: timestamppb.New(fixed.Add(-14 * 24 * time.Hour)),
	})

	if !strings.Contains(note, "the oldest price behind it is from 2026-08-09T06:00:00Z (6 hours ago)") {
		t.Errorf("prices_as_of must be dated and aged, got: %s", note)
	}
	if !strings.Contains(note, "the quantities were last confirmed 2026-07-26T12:00:00Z (14 days ago)") {
		t.Errorf("amounts_as_of must be spoken alongside it, got: %s", note)
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

// TestAge_SingularAtOne: production printed "1 hours ago" on the first live
// call. The note is quoted verbatim by a model, and a sentence that cannot
// count reads as machine output rather than as a claim.
func TestAge_SingularAtOne(t *testing.T) {
	fixed := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	now = func() time.Time { return fixed }
	t.Cleanup(func() { now = time.Now })

	cases := []struct {
		ago  time.Duration
		want string
	}{
		{time.Minute, "1 minute ago"},
		{2 * time.Minute, "2 minutes ago"},
		{time.Hour, "1 hour ago"},
		{6 * time.Hour, "6 hours ago"},
		{24 * time.Hour, "24 hours ago"}, // still hours below the 48h cut
		{48 * time.Hour, "2 days ago"},
		{72 * time.Hour, "3 days ago"},
	}
	for _, c := range cases {
		if got := age(fixed.Add(-c.ago)); got != c.want {
			t.Errorf("age(-%s) = %q, want %q", c.ago, got, c.want)
		}
	}
}

// TestCoverageNote_ProductionShapes pins the two shapes production actually
// produces, whole. The clauses are tested separately above; this one exists
// because the sentence is read by a model as one string, and a defect that only
// shows up in how the parts join would pass every test that checks a part.
func TestCoverageNote_ProductionShapes(t *testing.T) {
	fixed := time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	now = func() time.Time { return fixed }
	t.Cleanup(func() { now = time.Now })

	// The `stocks` portfolio on production: nine FinEx ETFs, every source asked
	// since 2026-08-07, nothing priced. This is the case that read as "no quote
	// at all" before, which was the opposite of what the data said.
	stocks := coverageNote(&apiv1.ValuationCoverage{
		UnpricedCount: 9,
		Unpriced: []*apiv1.UnpricedHolding{
			{Symbol: "FXRU", Reason: apiv1.UnpricedReason_UNPRICED_REASON_NEVER_PRICED},
			{Symbol: "FXGD", Reason: apiv1.UnpricedReason_UNPRICED_REASON_NEVER_PRICED},
			{Symbol: "FXRL", Reason: apiv1.UnpricedReason_UNPRICED_REASON_NEVER_PRICED},
			{Symbol: "FXUS", Reason: apiv1.UnpricedReason_UNPRICED_REASON_NEVER_PRICED},
			{Symbol: "FXDM", Reason: apiv1.UnpricedReason_UNPRICED_REASON_NEVER_PRICED},
			{Symbol: "FXIM", Reason: apiv1.UnpricedReason_UNPRICED_REASON_NEVER_PRICED},
			{Symbol: "FXWO", Reason: apiv1.UnpricedReason_UNPRICED_REASON_NEVER_PRICED},
			{Symbol: "FXTB", Reason: apiv1.UnpricedReason_UNPRICED_REASON_NEVER_PRICED},
			{Symbol: "FXCN", Reason: apiv1.UnpricedReason_UNPRICED_REASON_NEVER_PRICED},
		},
	})
	wantStocks := "Coverage: 0 of 9 holdings priced. 9 holding(s) are OUT of the total — of the 9 listed, " +
		"9 have been asked of every source available and never answered (evidence of silence, not a delisting verdict). " +
		"Any total from this result covers priced holdings only; say so when reporting it."
	if stocks != wantStocks {
		t.Errorf("stocks note:\n got: %s\nwant: %s", stocks, wantStocks)
	}

	// The `crypto` portfolio: a mixed, capped sample, priced quantities two weeks
	// old and prices from this morning — the case where dating one axis and not
	// the other would mislead.
	crypto := coverageNote(&apiv1.ValuationCoverage{
		PricedCount:   85,
		UnpricedCount: 71,
		Unpriced: []*apiv1.UnpricedHolding{
			{Symbol: "NFT", Reason: apiv1.UnpricedReason_UNPRICED_REASON_NEVER_PRICED},
			{Symbol: "TSTON", Reason: apiv1.UnpricedReason_UNPRICED_REASON_THIN_MARKET},
			{Symbol: "DYM", Reason: apiv1.UnpricedReason_UNPRICED_REASON_NO_QUOTE},
		},
		UnpricedTruncated: true,
		StaleCount:        2,
		PricesAsOf:        timestamppb.New(fixed.Add(-2 * time.Hour)),
		AmountsAsOf:       timestamppb.New(fixed.Add(-14 * 24 * time.Hour)),
	})
	wantCrypto := "Coverage: 85 of 156 holdings priced. 71 holding(s) are OUT of the total — of the 3 listed, " +
		"1 have no quote yet, 1 have been asked of every source available and never answered " +
		"(evidence of silence, not a delisting verdict), 1 have a quote with no market behind it; " +
		"the list is a capped sample of a larger set. " +
		"2 priced holding(s) are IN the total on a quote older than this instance's freshness policy — " +
		"they are named, not removed, so do not subtract them a second time. " +
		"Dating this total: the oldest price behind it is from 2026-08-09T06:00:00Z (2 hours ago), " +
		"the quantities were last confirmed 2026-07-26T08:00:00Z (14 days ago). " +
		"Any total from this result covers priced holdings only; say so when reporting it."
	if crypto != wantCrypto {
		t.Errorf("crypto note:\n got: %s\nwant: %s", crypto, wantCrypto)
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

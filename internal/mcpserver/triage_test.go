package mcpserver

import (
	"strings"
	"testing"

	apiv1 "github.com/foxcool/greedy-eye/api/v1"
)

func TestIsSettableVerdict(t *testing.T) {
	for _, v := range []string{"legit", "suspect", "scam", "impersonation"} {
		if !isSettableVerdict(v) {
			t.Errorf("%q must be settable", v)
		}
	}
	// "unknown" is a state the heuristic holds, not one a caller may set.
	for _, v := range []string{"unknown", "", "SCAM", "legitimate"} {
		if isSettableVerdict(v) {
			t.Errorf("%q must not be settable", v)
		}
	}
}

func TestIsQuarantineVerdict(t *testing.T) {
	for _, v := range []string{"scam", "impersonation"} {
		if !isQuarantineVerdict(v) {
			t.Errorf("%q must quarantine", v)
		}
	}
	for _, v := range []string{"legit", "suspect", "unknown", ""} {
		if isQuarantineVerdict(v) {
			t.Errorf("%q must not quarantine", v)
		}
	}
}

// The plan must describe the direction of the change, not just the new label:
// entering quarantine removes money from the total, leaving it does not put the
// same money back.
func TestVerdictEffect(t *testing.T) {
	tests := []struct {
		name    string
		current string
		next    string
		want    string
	}{
		{"into quarantine", "legit", "scam", "Quarantines"},
		{"into quarantine from unknown", "unknown", "impersonation", "Quarantines"},
		{"out of quarantine", "scam", "legit", "Withdraws"},
		{"within quarantine", "scam", "impersonation", "Stays quarantined"},
		{"outside quarantine", "unknown", "suspect", "No quarantine"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := verdictEffect(tt.current, tt.next)
			if !contains(got, tt.want) {
				t.Errorf("verdictEffect(%q, %q) = %q, want it to mention %q",
					tt.current, tt.next, got, tt.want)
			}
		})
	}
}

// Withdrawing a quarantine leaves the already-excluded rows excluded. A caller
// told only "verdict updated" would report a repair that did not happen.
func TestVerdictCommitNoteWarnsOnWithdrawal(t *testing.T) {
	note := verdictCommitNote("scam", "legit")
	if !contains(note, "NOT released") {
		t.Errorf("withdrawing a quarantine must warn that holdings stay excluded, got: %s", note)
	}

	if note := verdictCommitNote("legit", "scam"); !contains(note, "out of every total") {
		t.Errorf("quarantining must state that holdings leave totals, got: %s", note)
	}

	if note := verdictCommitNote("unknown", "suspect"); note != "" {
		t.Errorf("a change that crosses no quarantine boundary needs no note, got: %s", note)
	}
}

func TestPartitionRefs(t *testing.T) {
	refs := []*apiv1.AssetExternalRef{
		{Id: "a", Source: "onchain:eth", Ref: "0x1"},
		{Id: "b", Source: "onchain:polygon", Ref: "0x2"},
		{Id: "c", Source: "coingecko", Ref: "aave"},
	}

	target, remaining := partitionRefs(refs, "b")
	if target == nil || target.GetId() != "b" {
		t.Fatalf("expected ref b as the target, got %v", target)
	}
	if len(remaining) != 2 {
		t.Fatalf("expected 2 remaining refs, got %d", len(remaining))
	}
	for _, r := range remaining {
		if r.GetId() == "b" {
			t.Error("the target must not appear in the remainder")
		}
	}

	// An unknown id yields no target, and the remainder is the whole set: the
	// caller is then told what the asset is actually bound through.
	target, remaining = partitionRefs(refs, "zzz")
	if target != nil {
		t.Errorf("expected no target for an unknown id, got %v", target)
	}
	if len(remaining) != len(refs) {
		t.Errorf("expected all %d refs in the remainder, got %d", len(refs), len(remaining))
	}
}

func TestChainOfRef(t *testing.T) {
	tests := []struct {
		source    string
		wantChain string
		wantOK    bool
	}{
		{"onchain:polygon", "polygon", true},
		{"onchain:eth", "eth", true},
		{"coingecko", "", false},
		{"onchain:", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		chain, ok := chainOfRef(&apiv1.AssetExternalRef{Source: tt.source})
		if ok != tt.wantOK || chain != tt.wantChain {
			t.Errorf("chainOfRef(%q) = (%q, %v), want (%q, %v)",
				tt.source, chain, ok, tt.wantChain, tt.wantOK)
		}
	}
}

// The description carries the ref id because that is what the commit call needs,
// and the contract because that is what the user recognises.
func TestRefDescription(t *testing.T) {
	got := refDescription(&apiv1.AssetExternalRef{
		Id:     "019f-ref",
		Source: "onchain:polygon",
		Ref:    "0xd9503c33",
		Origin: "auto",
	})
	for _, want := range []string{"019f-ref", "onchain:polygon", "0xd9503c33", "auto"} {
		if !contains(got, want) {
			t.Errorf("refDescription must mention %q, got: %s", want, got)
		}
	}

	if refs := describeRefs(nil); len(refs) != 0 {
		t.Errorf("an asset with no bindings describes as an empty list, got %v", refs)
	}
}

// A raw amount is unreadable at 18 decimals; the plan shows what the user holds.
func TestDescribeHolding(t *testing.T) {
	h := &apiv1.Holding{
		Id:        "019f-hold",
		AssetId:   "019f-asset",
		AccountId: "019f-acct",
		Amount:    "617620646444969042",
		Decimals:  18,
		Chain:     "eth",
		Source:    apiv1.ProvenanceSource_PROVENANCE_SOURCE_SYNC,
		Excluded:  true,
	}
	got := describeHolding(h)

	if got["amount"] != "0.617620646444969042" {
		t.Errorf("amount must be scaled by decimals, got %v", got["amount"])
	}
	if got["chain"] != "eth" {
		t.Errorf("chain must be reported, got %v", got["chain"])
	}
	if got["excluded"] != true {
		t.Errorf("excluded must be reported, got %v", got["excluded"])
	}
	if _, ok := got["portfolio_id"]; ok {
		t.Error("an unset portfolio_id must be omitted rather than reported as empty")
	}

	// A manual row is the case where deletion is permanent, so provenance has to
	// survive into the plan.
	h.Source = apiv1.ProvenanceSource_PROVENANCE_SOURCE_MANUAL
	if got := describeHolding(h); got["source"] != "PROVENANCE_SOURCE_MANUAL" {
		t.Errorf("source must be reported, got %v", got["source"])
	}
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

package mcpserver

import (
	"strings"
	"testing"
	"time"

	apiv1 "github.com/foxcool/greedy-eye/api/v1"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/foxcool/greedy-eye-mcp/internal/backend"
	"github.com/foxcool/greedy-eye-mcp/internal/config"
)

// TestIncomeReportRequest_ExactlyOneTarget: the report is of a portfolio or an
// account, never both or neither, and the period is RFC 3339.
func TestIncomeReportRequest_ExactlyOneTarget(t *testing.T) {
	period := map[string]any{"from": "2026-11-01T00:00:00Z", "to": "2026-12-01T00:00:00Z"}
	with := func(extra map[string]any) map[string]any {
		out := map[string]any{}
		for k, v := range period {
			out[k] = v
		}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}
	for name, args := range map[string]map[string]any{
		"neither": with(nil),
		"both":    with(map[string]any{"portfolio_id": "p", "account_id": "a"}),
	} {
		if _, err := incomeReportRequest(callWith(args)); err == nil || !strings.Contains(err.Error(), "exactly one") {
			t.Errorf("%s: want an 'exactly one' error, got %v", name, err)
		}
	}
	if _, err := incomeReportRequest(callWith(with(map[string]any{"account_id": "a", "from": "2026-11-01"}))); err == nil ||
		!strings.Contains(err.Error(), "from") {
		t.Errorf("a date without time must be refused naming from, got %v", err)
	}

	in, err := incomeReportRequest(callWith(with(map[string]any{"account_id": "a", "quote_asset_id": "RUB"})))
	if err != nil {
		t.Fatalf("incomeReportRequest: %v", err)
	}
	if in.GetAccountId() != "a" || in.PortfolioId != nil || in.GetQuoteAssetId() != "RUB" {
		t.Errorf("request = %v", in)
	}
	if !in.GetFrom().AsTime().Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("from = %v", in.GetFrom().AsTime())
	}
}

// TestIncomeReportExtras_UnvaluedIsSpoken: an event with no price is in the
// amounts and not in the values; the note says the totals understate.
func TestIncomeReportExtras_UnvaluedIsSpoken(t *testing.T) {
	resp := &apiv1.GetIncomeReportResponse{
		ValueDecimals: 2,
		Rows: []*apiv1.IncomeReportRow{{
			Month: timestamppb.New(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)), IncomeKind: apiv1.IncomeKind_INCOME_KIND_INTEREST,
			Symbol: "RUB", GrossAmount: "10000", TaxAmount: "1300", NetAmount: "8700", Decimals: 2, NetValue: "105",
		}},
		GrossValue: "745", TaxValue: "16", NetValue: "729",
		// One event, its payment and its tax both unpriced: listed twice.
		Unvalued: []*apiv1.UnvaluedIncome{{EventId: "e1", Symbol: "USDT"}, {EventId: "e1", Symbol: "XYZ"}},
	}
	extras := incomeReportExtras(resp)
	if extras["net_value_human"] != "7.29" || extras["tax_value_human"] != "0.16" {
		t.Errorf("totals = %v / %v", extras["net_value_human"], extras["tax_value_human"])
	}
	row := extras["rows_human"].([]map[string]any)[0]
	if row["month"] != "2026-11" || row["income_kind"] != "interest" || row["gross"] != "100" || row["tax"] != "13" || row["net"] != "87" {
		t.Errorf("row = %v", row)
	}
	if note := extras["income_note"].(string); !strings.Contains(note, "1 income event(s) had no price") ||
		!strings.Contains(note, "understate") {
		t.Errorf("note = %s", note)
	}
}

func TestIncomeNote_AllValuedAndEmpty(t *testing.T) {
	if got := incomeNote(&apiv1.GetIncomeReportResponse{}); !strings.Contains(got, "No income") {
		t.Errorf("empty = %s", got)
	}
	full := &apiv1.GetIncomeReportResponse{Rows: []*apiv1.IncomeReportRow{{}}}
	if got := incomeNote(full); !strings.Contains(got, "Every income event") {
		t.Errorf("all valued = %s", got)
	}
}

// TestIncomeReportIsReadOnly: the report is registered without mutations.
func TestIncomeReportIsReadOnly(t *testing.T) {
	off := New(config.Config{}, &backend.Clients{}).ListTools()
	st, ok := off["eye_income_report"]
	if !ok {
		t.Fatal("eye_income_report missing with mutations off")
	}
	if a := st.Tool.Annotations; a.ReadOnlyHint == nil || !*a.ReadOnlyHint {
		t.Error("eye_income_report must be read-only")
	}
}

// TestSyncAccountTakesBackfillLedger: the full history read is a parameter of
// the sync tool, not a separate tool.
func TestSyncAccountTakesBackfillLedger(t *testing.T) {
	on := New(config.Config{EnableMutations: true}, &backend.Clients{}).ListTools()
	if _, ok := on["eye_sync_account"].Tool.InputSchema.Properties["backfill_ledger"]; !ok {
		t.Error("eye_sync_account has no backfill_ledger")
	}
}

// TestSyncNote_LedgerErrorsAreSpoken: a chain whose history was not brought up
// to date is named apart from the balance errors, which it does not affect.
func TestSyncNote_LedgerErrorsAreSpoken(t *testing.T) {
	note := syncNote(&apiv1.SyncAccountResponse{
		HoldingsUpserted:    5,
		LedgerEventsCreated: 1213,
		LedgerErrors:        []string{"chain base: alchemy: API status 403"},
	})
	for _, want := range []string{"1213 ledger event(s)", "1 chain(s) or movement(s)", "ledger_errors"} {
		if !strings.Contains(note, want) {
			t.Errorf("sync note must contain %q, got: %s", want, note)
		}
	}
	if strings.Contains(note, "per-item error") {
		t.Errorf("ledger errors are not balance errors, got: %s", note)
	}
}

// TestParseProtoArrayNormalizesIncomeKind: income_kind takes the short name.
func TestParseProtoArrayNormalizesIncomeKind(t *testing.T) {
	items, err := parseProtoArray(
		`[{"kind":"income","income_kind":"staking_reward","executed_at":"2026-11-03T08:00:00Z",`+
			`"legs":[{"symbol":"USDT","direction":"in","role":"income","amount":"1"}]}]`,
		func() *apiv1.ImportTransactionItem { return &apiv1.ImportTransactionItem{} },
	)
	if err != nil {
		t.Fatalf("parseProtoArray: %v", err)
	}
	if items[0].IncomeKind != apiv1.IncomeKind_INCOME_KIND_STAKING_REWARD {
		t.Errorf("income_kind = %v", items[0].IncomeKind)
	}
}

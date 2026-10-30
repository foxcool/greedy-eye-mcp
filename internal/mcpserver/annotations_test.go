package mcpserver

import (
	"testing"

	"github.com/foxcool/greedy-eye-mcp/internal/backend"
	"github.com/foxcool/greedy-eye-mcp/internal/config"
)

type toolHints struct {
	readOnly, destructive, idempotent bool
}

var (
	readHints = toolHints{readOnly: true, idempotent: true}

	// toolClassification is the reviewed read/write table. A new tool must be
	// added here: the test fails on any registered tool it does not list.
	toolClassification = map[string]toolHints{
		"eye_list_portfolios":           readHints,
		"eye_get_portfolio":             readHints,
		"eye_list_accounts":             readHints,
		"eye_list_holdings":             readHints,
		"eye_list_unpriced_holdings":    readHints,
		"eye_calculate_portfolio_value": readHints,
		"eye_get_account_health":        readHints,
		"eye_list_assets":               readHints,
		"eye_get_asset":                 readHints,
		"eye_get_latest_price":          readHints,
		"eye_list_price_history":        readHints,
		"eye_get_sweep_schedule":        readHints,
		"eye_get_heatmap":               readHints,
		"eye_list_rules":                readHints,
		"eye_get_rule":                  readHints,
		"eye_simulate_rule":             readHints,

		// Additive writes.
		"eye_create_manual_account": {},
		"eye_find_or_create_asset":  {idempotent: true},
		"eye_import_transactions":   {idempotent: true},
		"eye_fetch_external_prices": {},

		// Writes that delete or overwrite existing state.
		"eye_import_positions":     {destructive: true},
		"eye_sync_account":         {destructive: true},
		"eye_reset_sweep_schedule": {destructive: true, idempotent: true},
		"eye_unbind_asset_ref":     {destructive: true, idempotent: true},
		"eye_set_asset_verdict":    {destructive: true, idempotent: true},
		"eye_delete_holding":       {destructive: true, idempotent: true},
	}
)

func TestToolAnnotations(t *testing.T) {
	// Mutations on, so every tool the binary can expose is registered.
	s := New(config.Config{EnableMutations: true}, &backend.Clients{})
	tools := s.ListTools()

	for name, st := range tools {
		want, ok := toolClassification[name]
		if !ok {
			t.Errorf("%s: not classified in toolClassification", name)
			continue
		}
		a := st.Tool.Annotations
		check := func(hint string, got *bool, want bool) {
			if got == nil {
				t.Errorf("%s: %s not set", name, hint)
			} else if *got != want {
				t.Errorf("%s: %s = %v, want %v", name, hint, *got, want)
			}
		}
		check("readOnlyHint", a.ReadOnlyHint, want.readOnly)
		check("destructiveHint", a.DestructiveHint, want.destructive)
		check("idempotentHint", a.IdempotentHint, want.idempotent)
	}

	for name := range toolClassification {
		if _, ok := tools[name]; !ok {
			t.Errorf("%s: classified but not registered", name)
		}
	}
}

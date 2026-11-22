package mcpserver

import (
	"strings"
	"testing"

	"github.com/foxcool/greedy-eye-mcp/internal/backend"
	"github.com/foxcool/greedy-eye-mcp/internal/config"
)

// writeToolNames are the tools that write to the backend (or spend provider
// quota); none may be registered unless mutations are enabled.
var writeToolNames = []string{
	"eye_create_manual_account", "eye_find_or_create_asset",
	"eye_import_positions", "eye_import_transactions",
	"eye_reset_sweep_schedule", "eye_unbind_asset_ref",
	"eye_set_asset_verdict", "eye_delete_holding", "eye_merge_assets",
	"eye_fetch_external_prices", "eye_sync_account",
}

// The instructions promise dry_run=true by default and name the write tools
// that act at once. A write tool without a dry_run parameter that the text does
// not name turns a client's "first call is a plan" into a real write.
func TestInstructionsNameEveryWriteToolWithoutDryRun(t *testing.T) {
	on := New(config.Config{EnableMutations: true}, &backend.Clients{}).ListTools()
	for _, name := range writeToolNames {
		st, ok := on[name]
		if !ok {
			t.Fatalf("%s missing with mutations on", name)
		}
		_, hasDryRun := st.Tool.InputSchema.Properties["dry_run"]
		named := strings.Contains(serverInstructions, name)
		if !hasDryRun && !named {
			t.Errorf("%s has no dry_run but the instructions do not name it as acting at once", name)
		}
		if hasDryRun && named {
			t.Errorf("%s has dry_run but the instructions name it as acting at once", name)
		}
	}
}

func TestToolSetFollowsMutationsGate(t *testing.T) {
	off := New(config.Config{}, &backend.Clients{}).ListTools()
	on := New(config.Config{EnableMutations: true}, &backend.Clients{}).ListTools()

	for _, name := range writeToolNames {
		if _, ok := off[name]; ok {
			t.Errorf("%s registered with mutations off", name)
		}
		if _, ok := on[name]; !ok {
			t.Errorf("%s missing with mutations on", name)
		}
	}
	// Everything registered with mutations off must be a read-only tool.
	for name, st := range off {
		if a := st.Tool.Annotations; a.ReadOnlyHint == nil || !*a.ReadOnlyHint {
			t.Errorf("%s registered with mutations off but is not read-only", name)
		}
	}
	if len(off) == 0 || len(on) != len(off)+len(writeToolNames) {
		t.Errorf("tool counts: off=%d on=%d, want on = off + %d", len(off), len(on), len(writeToolNames))
	}
}

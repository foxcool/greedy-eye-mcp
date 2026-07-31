package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"

	apiv1 "github.com/foxcool/greedy-eye/api/v1"
)

func TestEnumValue(t *testing.T) {
	assetType := apiv1.AssetType_ASSET_TYPE_UNSPECIFIED.Descriptor()

	tests := []struct {
		name string
		raw  string
		want apiv1.AssetType
	}{
		{"empty means absent", "", apiv1.AssetType_ASSET_TYPE_UNSPECIFIED},
		{"short form", "fund", apiv1.AssetType_ASSET_TYPE_FUND},
		{"short form mixed case", "Stock", apiv1.AssetType_ASSET_TYPE_STOCK},
		{"full member name", "ASSET_TYPE_BOND", apiv1.AssetType_ASSET_TYPE_BOND},
		{"full name lowercased", "asset_type_commodity", apiv1.AssetType_ASSET_TYPE_COMMODITY},
		{"alias etf resolves to fund", "etf", apiv1.AssetType_ASSET_TYPE_FUND},
		{"alias is case-insensitive", "ETF", apiv1.AssetType_ASSET_TYPE_FUND},
		{"surrounding space", "  cryptocurrency  ", apiv1.AssetType_ASSET_TYPE_CRYPTOCURRENCY},
		{"hyphen separator", "asset-type-bond", apiv1.AssetType_ASSET_TYPE_BOND},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := enumValue[apiv1.AssetType](assetType, "type", tt.raw)
			if err != nil {
				t.Fatalf("enumValue(%q) unexpected error: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("enumValue(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestEnumValueRejects(t *testing.T) {
	assetType := apiv1.AssetType_ASSET_TYPE_UNSPECIFIED.Descriptor()

	// "unspecified" is not selectable: the backend reads the zero value as
	// "default to cryptocurrency", so accepting it would create a crypto asset
	// under a wrong identity — the exact silent fallback this helper replaces.
	for _, raw := range []string{"etfs", "equity", "ASSET_TYPE_ETF", "unspecified", "0"} {
		t.Run(raw, func(t *testing.T) {
			got, err := enumValue[apiv1.AssetType](assetType, "type", raw)
			if err == nil {
				t.Fatalf("enumValue(%q) = %v, want an error", raw, got)
			}
			if got != apiv1.AssetType_ASSET_TYPE_UNSPECIFIED {
				t.Fatalf("enumValue(%q) returned %v alongside the error", raw, got)
			}
			for _, want := range []string{raw, "type", "cryptocurrency", "stock", "bond", "commodity", "forex", "fund", "etf"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
		})
	}
}

func TestParseProtoArrayNormalizesEnums(t *testing.T) {
	raw := `[
		{"symbol":"FXUS","amount":"10","market":"spbex","asset_type":"etf"},
		{"symbol":"AAPL","amount":"1","market":"nasdaq","assetType":"ASSET_TYPE_STOCK"},
		{"symbol":"BTC","amount":"0.5"}
	]`
	items, err := parseProtoArray(raw, func() *apiv1.ImportPositionItem { return &apiv1.ImportPositionItem{} })
	if err != nil {
		t.Fatalf("parseProtoArray: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}
	want := []apiv1.AssetType{
		apiv1.AssetType_ASSET_TYPE_FUND,
		apiv1.AssetType_ASSET_TYPE_STOCK,
		apiv1.AssetType_ASSET_TYPE_UNSPECIFIED, // absent: the backend defaults it
	}
	for i, item := range items {
		if item.AssetType != want[i] {
			t.Errorf("item %d asset_type = %v, want %v", i, item.AssetType, want[i])
		}
	}
	if got := items[0].GetMarket(); got != "spbex" {
		t.Errorf("normalization altered a non-enum field: market = %q", got)
	}
}

func TestParseProtoArrayReportsValidEnumValues(t *testing.T) {
	_, err := parseProtoArray(
		`[{"symbol":"FXUS","amount":"10","asset_type":"equity"}]`,
		func() *apiv1.ImportPositionItem { return &apiv1.ImportPositionItem{} },
	)
	if err == nil {
		t.Fatal("expected an error for an unknown asset type")
	}
	for _, want := range []string{"item 0", "asset_type", "equity", "fund", "etf"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestParseProtoArrayNormalizesTransactionType(t *testing.T) {
	items, err := parseProtoArray(
		`[{"type":"deposit","symbol":"BTC"},{"type":"TRANSACTION_TYPE_TRADE","symbol":"ETH"}]`,
		func() *apiv1.ImportTransactionItem { return &apiv1.ImportTransactionItem{} },
	)
	if err != nil {
		t.Fatalf("parseProtoArray: %v", err)
	}
	want := []apiv1.TransactionType{
		apiv1.TransactionType_TRANSACTION_TYPE_DEPOSIT,
		apiv1.TransactionType_TRANSACTION_TYPE_TRADE,
	}
	for i, item := range items {
		if item.Type != want[i] {
			t.Errorf("item %d type = %v, want %v", i, item.Type, want[i])
		}
	}
}

func TestNormalizeEnumFieldsLeavesNonObjectsAlone(t *testing.T) {
	md := (&apiv1.ImportPositionItem{}).ProtoReflect().Descriptor()
	raw := json.RawMessage(`"not an object"`)
	got, err := normalizeEnumFields(md, raw)
	if err != nil {
		t.Fatalf("normalizeEnumFields: %v", err)
	}
	if string(got) != string(raw) {
		t.Fatalf("got %s, want the input unchanged", got)
	}
}

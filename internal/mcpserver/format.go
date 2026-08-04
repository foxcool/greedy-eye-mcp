package mcpserver

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	apiv1 "github.com/foxcool/greedy-eye/api/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// protoJSON renders a protobuf message as readable JSON using the proto field
// names (snake_case), so the model sees the same shape as the .proto schema.
var protoJSON = protojson.MarshalOptions{
	UseProtoNames:   true,
	EmitUnpopulated: false,
	Indent:          "  ",
}

// resultProto marshals a protobuf response message into a tool text result.
func resultProto(msg proto.Message) (*mcp.CallToolResult, error) {
	b, err := protoJSON.Marshal(msg)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode response: %v", err)), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}

// resultProtoWith marshals a protobuf response and adds derived fields to the
// top level of the JSON object — a human-readable total, a coverage sentence.
// A field the model has to compute for itself is a field it reports wrong.
//
// Falls back to the plain proto rendering if either encoding step fails: the
// response itself is worth more than the enrichment.
func resultProtoWith(msg proto.Message, extra map[string]any) (*mcp.CallToolResult, error) {
	raw, err := protoJSON.Marshal(msg)
	if err != nil {
		return resultProto(msg)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return resultProto(msg)
	}
	for k, v := range extra {
		if s, ok := v.(string); ok && s == "" {
			continue
		}
		m[k] = v
	}
	return resultJSON(m)
}

// coverageNote states, in words, how much of a valuation actually had prices
// behind it.
//
// The counts are in the response already, but a number in a nested JSON field is
// easy to summarise past: an assistant reading `total_value` reports a total,
// and if part of the portfolio silently stayed out of it, the user hears a
// complete answer to a question that was only partly answered. The sentence
// exists to make that impossible to miss, and it says what to do about it.
//
// The reason breakdown counts only the disclosed sample, which is capped, so it
// is worded as a sample rather than as a total.
func coverageNote(cov *apiv1.ValuationCoverage) string {
	if cov == nil {
		return ""
	}
	if cov.GetUnpricedCount() == 0 {
		if cov.GetPricedCount() == 0 {
			return "No holdings in scope: this result values nothing."
		}
		return fmt.Sprintf("Coverage: all %d holdings were priced; nothing is missing from this result.",
			cov.GetPricedCount())
	}

	var noQuote, thin int
	for _, u := range cov.GetUnpriced() {
		switch u.GetReason() {
		case apiv1.UnpricedReason_UNPRICED_REASON_THIN_MARKET:
			thin++
		default:
			noQuote++
		}
	}
	reasons := fmt.Sprintf("%d of the %d listed have no quote at all, %d have a quote with no market behind it",
		noQuote, len(cov.GetUnpriced()), thin)
	if cov.GetUnpricedTruncated() {
		reasons += "; the list is a capped sample of a larger set"
	}

	return fmt.Sprintf(
		"Coverage: %d of %d holdings priced. %d holding(s) are NOT included — %s. "+
			"Any total from this result covers priced holdings only; say so when reporting it.",
		cov.GetPricedCount(), cov.GetPricedCount()+cov.GetUnpricedCount(), cov.GetUnpricedCount(), reasons)
}

// resultJSON marshals an arbitrary value (typically an enriched map) into a result.
func resultJSON(v any) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode response: %v", err)), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}

// scaledDecimal converts a raw integer string scaled by `decimals` into a
// human-readable decimal string. greedy-eye stores balances and prices this way
// because on-chain uint256 values overflow int64. Returns the input unchanged
// if it is not a plain integer.
func scaledDecimal(raw string, decimals uint32) string {
	if raw == "" {
		return ""
	}
	n, ok := new(big.Int).SetString(raw, 10)
	if !ok {
		return raw
	}
	if decimals == 0 {
		return n.String()
	}

	neg := n.Sign() < 0
	digits := new(big.Int).Abs(n).String()
	d := int(decimals)
	if len(digits) <= d {
		digits = strings.Repeat("0", d-len(digits)+1) + digits
	}

	intPart := digits[:len(digits)-d]
	fracPart := strings.TrimRight(digits[len(digits)-d:], "0")

	out := intPart
	if fracPart != "" {
		out += "." + fracPart
	}
	if neg {
		out = "-" + out
	}
	return out
}

// parseTimestamp parses an RFC3339 string into a protobuf Timestamp.
func parseTimestamp(s string) (*timestamppb.Timestamp, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, fmt.Errorf("expected RFC3339 timestamp, got %q: %w", s, err)
	}
	return timestamppb.New(t), nil
}

// optString returns a pointer to s, or nil if s is empty. Useful for proto3
// `optional string` request fields where empty means "unset".
func optString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// optInt32 returns a pointer to int32(i), or nil if i == 0. The value is clamped
// to the int32 range so an out-of-range page size can never silently overflow
// (gosec G115).
func optInt32(i int) *int32 {
	if i == 0 {
		return nil
	}
	if i > math.MaxInt32 {
		i = math.MaxInt32
	} else if i < math.MinInt32 {
		i = math.MinInt32
	}
	v := int32(i)
	return &v
}

package mcpserver

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	apiv1 "github.com/foxcool/greedy-eye/api/v1"
	"github.com/mark3labs/mcp-go/mcp"
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

// now is the clock the note dates itself against. A package variable so tests
// can pin it: an age rendered from the real clock is untestable.
var now = time.Now

// coverageNote states, in words, how much of a valuation actually had prices
// behind it and how much of it rests on prices that have gone quiet.
//
// The counts are in the response already, but a number in a nested JSON field is
// easy to summarise past: an assistant reading `total_value` reports a total,
// and if part of the portfolio silently stayed out of it, the user hears a
// complete answer to a question that was only partly answered. The sentence
// exists to make that impossible to miss, and it says what to do about it.
//
// Two kinds of doubt live here and they must never read as one. Unpriced
// holdings are OUT of the total: naming them tells the reader the total is
// short. Stale ones are IN it: they are named because the quote behind them is
// old, not because it is absent, and a reader who confuses the two subtracts
// the same doubt twice. Hence the explicit OUT/IN wording.
//
// Both dates are spoken whenever they exist, for the same reason: an unmentioned
// date reads as "current", and prices and quantities go stale independently.
//
// The reason breakdown counts only the disclosed sample, which is capped, so it
// is worded as a sample rather than as a total.
func coverageNote(cov *apiv1.ValuationCoverage) string {
	if cov == nil {
		return ""
	}
	if cov.GetUnpricedCount() == 0 && cov.GetPricedCount() == 0 {
		return "No holdings in scope: this result values nothing."
	}

	var parts []string
	if cov.GetUnpricedCount() == 0 {
		parts = append(parts, fmt.Sprintf(
			"Coverage: all %d holdings were priced; nothing is missing from this result.",
			cov.GetPricedCount()))
	} else {
		parts = append(parts, fmt.Sprintf(
			"Coverage: %d of %d holdings priced. %d holding(s) are OUT of the total — %s.",
			cov.GetPricedCount(), cov.GetPricedCount()+cov.GetUnpricedCount(),
			cov.GetUnpricedCount(), unpricedReasons(cov)))
	}

	if s := stalePhrase(cov); s != "" {
		parts = append(parts, s)
	}
	if d := datesPhrase(cov); d != "" {
		parts = append(parts, d)
	}
	if cov.GetUnpricedCount() > 0 {
		parts = append(parts, "Any total from this result covers priced holdings only; say so when reporting it.")
	}
	return strings.Join(parts, " ")
}

// unpricedReasons breaks the disclosed sample down by why each holding stayed
// out. Only non-empty buckets are spoken: a "0 have X" clause invites the reader
// to treat the absent case as meaningful.
//
// NEVER_PRICED is its own clause and must not fold into "no quote yet". They are
// different claims about where the gap lives — one says our pipeline has not
// reached the asset, the other says every source it has was asked and none ever
// answered. Collapsing them was a real defect: production reported nine ETFs as
// "no quote at all" while the field beside it said they had been asked since
// 2026-08-07.
func unpricedReasons(cov *apiv1.ValuationCoverage) string {
	var noQuote, thin, never int
	for _, u := range cov.GetUnpriced() {
		switch u.GetReason() {
		case apiv1.UnpricedReason_UNPRICED_REASON_THIN_MARKET:
			thin++
		case apiv1.UnpricedReason_UNPRICED_REASON_NEVER_PRICED:
			never++
		default:
			noQuote++
		}
	}

	var clauses []string
	if noQuote > 0 {
		clauses = append(clauses, fmt.Sprintf("%d have no quote yet", noQuote))
	}
	if never > 0 {
		clauses = append(clauses, fmt.Sprintf(
			"%d have been asked of every source available and never answered (evidence of silence, not a delisting verdict)", never))
	}
	if thin > 0 {
		clauses = append(clauses, fmt.Sprintf("%d have a quote with no market behind it", thin))
	}

	out := fmt.Sprintf("of the %d listed, %s", len(cov.GetUnpriced()), strings.Join(clauses, ", "))
	if len(clauses) == 0 {
		out = fmt.Sprintf("the %d listed carry no stated reason", len(cov.GetUnpriced()))
	}
	if cov.GetUnpricedTruncated() {
		out += "; the list is a capped sample of a larger set"
	}
	return out
}

// stalePhrase speaks the priced holdings whose quote is older than the
// instance's freshness policy. Silent when none are: a "0 stale" clause is noise
// that trains the reader to skip the sentence on the day it matters.
func stalePhrase(cov *apiv1.ValuationCoverage) string {
	if cov.GetStaleCount() == 0 {
		return ""
	}
	return fmt.Sprintf(
		"%d priced holding(s) are IN the total on a quote older than this instance's freshness policy — "+
			"they are named, not removed, so do not subtract them a second time.",
		cov.GetStaleCount())
}

// datesPhrase dates both axes of the total. A price and an amount go stale
// independently and only the price has a sweep watching it, so naming one and
// omitting the other invites the reader to assume the omitted one is current.
func datesPhrase(cov *apiv1.ValuationCoverage) string {
	var clauses []string
	if ts := cov.GetPricesAsOf(); ts.IsValid() {
		clauses = append(clauses, fmt.Sprintf("the oldest price behind it is from %s (%s)",
			ts.AsTime().UTC().Format(time.RFC3339), age(ts.AsTime())))
	}
	if ts := cov.GetAmountsAsOf(); ts.IsValid() {
		clauses = append(clauses, fmt.Sprintf("the quantities were last confirmed %s (%s)",
			ts.AsTime().UTC().Format(time.RFC3339), age(ts.AsTime())))
	}
	if len(clauses) == 0 {
		return ""
	}
	return "Dating this total: " + strings.Join(clauses, ", ") + "."
}

// age renders how long ago t was, coarsely. The exact figure is in the
// timestamp beside it; this is the part a reader acts on.
func age(t time.Time) string {
	d := now().Sub(t)
	switch {
	case d < 0:
		return "in the future"
	case d < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
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

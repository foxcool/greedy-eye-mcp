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

// unpricedPageNote says in words whether this page is the end of the walk.
//
// The rows alone cannot say it: a short page is ordinary here, because the tail
// is recomputed live and a holding priced since the last call simply stops
// appearing. Only an empty next_page_token ends the walk, and a reader who
// infers completeness from row count would announce a partial worklist as the
// whole one.
func unpricedPageNote(rows int, nextToken string) string {
	if rows == 0 && nextToken == "" {
		return "No unpriced holdings match: every position in scope has a usable quote."
	}
	if nextToken == "" {
		return fmt.Sprintf("%d holding(s) on this page, and this is the last page: the walk is complete.", rows)
	}
	return fmt.Sprintf(
		"%d holding(s) on this page, and MORE REMAIN — call again with page_token=%q. "+
			"Do not report this page as the full set. A page can also be shorter than asked, "+
			"since a holding priced since the last call drops out of the walk.", rows, nextToken)
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
		// Naming the way out matters more than admitting the cap. "A capped
		// sample" told a reader the tail existed and left them no route to it,
		// so the tail went unworked — which is what the list is for.
		out += "; the list is a capped sample of a larger set — use eye_list_unpriced_holdings to walk all of them"
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
	} else if cov.GetPricedCount() > 0 {
		// The date covers synced amounts only, so it is absent when every counted
		// holding was entered by hand. Saying nothing here would read as "the
		// quantities are current" — the omission this whole sentence exists to
		// prevent — while a date would claim a confirmation nobody made.
		clauses = append(clauses, "no synced amount stands behind it — every holding in it was entered by hand, "+
			"so the quantities are as current as whoever last edited them")
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
		return plural(int(d.Minutes()), "minute") + " ago"
	case d < 48*time.Hour:
		return plural(int(d.Hours()), "hour") + " ago"
	default:
		return plural(int(d.Hours()/24), "day") + " ago"
	}
}

// plural renders a count with its unit, singular at one. The sentence this
// feeds is meant to be quoted verbatim by a model, and "1 hours ago" reads as
// output rather than as a statement someone stands behind.
func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
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

// syncNote says in words what a sync did and, more importantly, what it could
// not account for.
//
// The counts exist because a skip used to be silence: a position the source DID
// report and the instance could not name left no trace, and a snapshot missing
// one paper looks exactly like a snapshot missing nothing. The proto encoding
// drops a zero, so reading the counts off the response would make "nothing was
// skipped" indistinguishable from "this build does not count skips" — which is
// the ambiguity the fields were added to end. They are spoken here either way.
//
// The three are NOT symmetric and the note says so, because acting on them
// differs: a skip HOLDS BACK removals (paper that fell out of the catalogue is
// still owned, so the snapshot may not zero it), a defaulted market is a guess
// that holds nothing back, and a created account is neither — it is the fan-out
// of a broker credential reporting what it found.
func syncNote(resp *apiv1.SyncAccountResponse) string {
	if resp == nil {
		return ""
	}

	parts := []string{fmt.Sprintf(
		"Wrote %d holding(s) and zeroed %d; %d asset(s) upserted.",
		resp.GetHoldingsUpserted(), resp.GetHoldingsZeroed(), resp.GetAssetsUpserted())}

	if n := resp.GetPositionsSkipped(); n > 0 {
		parts = append(parts, fmt.Sprintf(
			"%d position(s) the source reported could NOT be named, so this snapshot does not "+
				"speak for them and no holding was removed on its word. Report the number: the "+
				"holdings it wrote look complete either way.", n))
	} else {
		parts = append(parts, "No position was skipped: the snapshot speaks for everything the source reported.")
	}

	if n := resp.GetAssetsDefaultedMarket(); n > 0 {
		parts = append(parts, fmt.Sprintf(
			"%d asset(s) were filed under a market GUESSED from the row's currency rather than "+
				"resolved. The guess holds nothing back, and it is the one place this work "+
				"guesses at all.", n))
	}

	if n := resp.GetAccountsCreated(); n > 0 {
		parts = append(parts, fmt.Sprintf(
			"%d account(s) were created: a broker credential reaches several brokerage accounts "+
				"and each gets its own local account, never merged. Two of them holding the same "+
				"share are two positions, and a transfer between them is an event.", n))
	}

	if errs := resp.GetErrors(); len(errs) > 0 {
		parts = append(parts, fmt.Sprintf(
			"%d per-item error(s): the snapshot landed but could not vouch for every balance in it.", len(errs)))
	}

	return strings.Join(parts, " ")
}

package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/foxcool/greedy-eye-mcp/internal/backend"
	apiv1 "github.com/foxcool/greedy-eye/api/v1"
)

// Asset triage: the tools that let a caller act on an identity error instead of
// only reading one.
//
// These wrap three backend RPCs that have no dry_run of their own, unlike the
// import path where the server plans the batch. The plan is therefore built
// here, out of reads the caller could have made itself: what the binding is,
// what the verdict is now, what the holding holds. That is worth doing rather
// than skipping, because the alternative is a mutation whose consequences are
// only visible after it happens — and every one of these mutations is about
// repairing exactly that kind of surprise.
//
// A plan built from reads is weaker than one the server computes: state can
// move between the plan and the commit. The commit call carries the same
// identifiers, so a stale plan fails on the identifier rather than acting on
// something else.

const (
	// onchainRefPrefix marks an external ref whose namespace is a chain, as in
	// "onchain:polygon". The suffix is comparable to Holding.chain.
	onchainRefPrefix = "onchain:"

	// syncRewriteWarning is repeated wherever a caller might reach for a
	// deletion that the next sync will silently undo.
	syncRewriteWarning = "This holding came from sync. Deleting it is not a repair: " +
		"the wallet really holds the token, so the next sync writes the row back. " +
		"To keep a counterfeit out of the total, unbind its contract " +
		"(eye_unbind_asset_ref) or mark the asset (eye_set_asset_verdict)."
)

// registerTriageTools wires the asset-triage write tools. Registered under the
// same ENABLE_MUTATIONS gate as the other write tools.
func registerTriageTools(s *server.MCPServer, c *backend.Clients) {
	registerUnbindAssetRef(s, c)
	registerSetAssetVerdict(s, c)
	registerDeleteHolding(s, c)
}

// registerUnbindAssetRef exposes DeleteAssetExternalRef: the repair primitive
// for a contract bound to the wrong asset.
func registerUnbindAssetRef(s *server.MCPServer, c *backend.Clients) {
	s.AddTool(
		mcp.NewTool("eye_unbind_asset_ref",
			mcp.WithDescription("Detach one external identifier (a contract, a mint, a coin id) from an asset. "+
				"Use when a contract is bound to an asset it is not: a counterfeit token that merged into a "+
				"real ticker inherits that asset's price and enters the total as real money. Read the bindings "+
				"first with eye_get_asset, which lists external_refs with their ids. "+
				"Simulation-first: dry_run defaults to true and answers with the binding that would be removed "+
				"and what remains; show that to the user and repeat with dry_run=false to commit. "+
				"This does NOT reassign holdings and does NOT change the total by itself — it frees the "+
				"contract, and the next eye_sync_account of the affected accounts resolves it on its own "+
				"merits and zeroes the stale row. Admin-only: a binding is catalogue identity, shared by "+
				"every user. Unbinding is not a verdict: it says this contract is not that asset, and leaves "+
				"the real asset alone."),
			mcp.WithString("asset_id", mcp.Required(), mcp.Description("Asset UUID the binding is attached to.")),
			mcp.WithString("ref_id", mcp.Required(), mcp.Description(
				"External ref UUID to remove, from Asset.external_refs[].id (eye_get_asset).")),
			mcp.WithBoolean("dry_run", mcp.Description(
				"Plan without writing. Defaults to true — pass false only to commit a confirmed plan.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			assetID, err := req.RequireString("asset_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			refID, err := req.RequireString("ref_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			asset, err := c.MarketData.GetAsset(ctx, connect.NewRequest(&apiv1.GetAssetRequest{Id: assetID}))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			target, remaining := partitionRefs(asset.Msg.GetExternalRefs(), refID)
			if target == nil {
				return mcp.NewToolResultError(fmt.Sprintf(
					"asset %s has no external ref %s. Its bindings are: %s",
					assetID, refID, describeRefs(asset.Msg.GetExternalRefs()))), nil
			}

			if req.GetBool("dry_run", true) {
				plan, err := planUnbind(ctx, c, asset.Msg, target, remaining)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				return resultJSON(plan)
			}

			_, err = c.MarketData.DeleteAssetExternalRef(ctx, connect.NewRequest(&apiv1.DeleteAssetExternalRefRequest{
				AssetId: assetID,
				Id:      refID,
			}))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			return resultJSON(map[string]any{
				"unbound":        refDescription(target),
				"asset":          assetLabel(asset.Msg),
				"remaining_refs": describeRefs(remaining),
				"next_step": "Nothing has left the total yet. Run eye_sync_account on the accounts holding " +
					"this asset: the freed contract resolves on its own merits, its balance lands on a " +
					"separate asset, and the stale row is zeroed by the snapshot.",
			})
		},
	)
}

// registerSetAssetVerdict exposes SetAssetVerdict: the tool that ends an
// argument with the heuristic.
func registerSetAssetVerdict(s *server.MCPServer, c *backend.Clients) {
	s.AddTool(
		mcp.NewTool("eye_set_asset_verdict",
			mcp.WithDescription("Set the identity verdict on an asset by hand. A user verdict is TERMINAL: "+
				"rescoring never overwrites it, so this ends the automatic judgement rather than nudging it. "+
				"scam and impersonation quarantine the asset — every holding of it leaves every total, on all "+
				"surfaces. legit and suspect do not quarantine. "+
				"Simulation-first: dry_run defaults to true and answers with the current verdict, what the new "+
				"one changes, and how many holdings are affected; show that and repeat with dry_run=false. "+
				"This is NOT the tool for a counterfeit that stole a real ticker — quarantining there marks "+
				"the whole asset, including the genuine holdings merged into it. Unbind the contract instead "+
				"(eye_unbind_asset_ref). Use a verdict when the asset itself is what it is judged to be."),
			mcp.WithString("asset_id", mcp.Required(), mcp.Description("Asset UUID.")),
			mcp.WithString("verdict", mcp.Required(), mcp.Description(
				"Verdict to set. scam and impersonation exclude every holding of this asset from every total."),
				mcp.Enum("legit", "suspect", "scam", "impersonation")),
			mcp.WithBoolean("dry_run", mcp.Description(
				"Plan without writing. Defaults to true — pass false only to commit a confirmed plan.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			assetID, err := req.RequireString("asset_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			verdict, err := req.RequireString("verdict")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if !isSettableVerdict(verdict) {
				return mcp.NewToolResultError(fmt.Sprintf(
					"verdict must be one of legit, suspect, scam, impersonation; got %q", verdict)), nil
			}

			asset, err := c.MarketData.GetAsset(ctx, connect.NewRequest(&apiv1.GetAssetRequest{Id: assetID}))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			if req.GetBool("dry_run", true) {
				plan, err := planVerdict(ctx, c, asset.Msg, verdict)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				return resultJSON(plan)
			}

			resp, err := c.MarketData.SetAssetVerdict(ctx, connect.NewRequest(&apiv1.SetAssetVerdictRequest{
				AssetId: assetID,
				Verdict: verdict,
			}))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return resultProtoWith(resp.Msg, map[string]any{
				"note": verdictCommitNote(asset.Msg.GetIdentityVerdict(), verdict),
			})
		},
	)
}

// registerDeleteHolding exposes DeleteHolding, with the warning that makes it
// the wrong first reach for a synced position.
func registerDeleteHolding(s *server.MCPServer, c *backend.Clients) {
	s.AddTool(
		mcp.NewTool("eye_delete_holding",
			mcp.WithDescription("Delete one holding row. Appropriate for a manual or imported position entered "+
				"in error. Almost never the right repair for a SYNCED position: the wallet really holds the "+
				"token, so the next sync writes the row back and the fix reads as successful while changing "+
				"nothing. For a counterfeit that inherited a real asset's price, unbind its contract "+
				"(eye_unbind_asset_ref); for an asset that is what it is judged to be, set a verdict "+
				"(eye_set_asset_verdict). "+
				"Simulation-first: dry_run defaults to true and answers with the row that would be deleted "+
				"and whether sync would restore it; show that and repeat with dry_run=false."),
			mcp.WithString("holding_id", mcp.Required(), mcp.Description("Holding UUID.")),
			mcp.WithBoolean("dry_run", mcp.Description(
				"Plan without writing. Defaults to true — pass false only to commit a confirmed plan.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			holdingID, err := req.RequireString("holding_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			holding, err := c.Portfolio.GetHolding(ctx, connect.NewRequest(&apiv1.GetHoldingRequest{Id: holdingID}))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			synced := holding.Msg.GetSource() == apiv1.ProvenanceSource_PROVENANCE_SOURCE_SYNC
			if req.GetBool("dry_run", true) {
				plan := map[string]any{
					"action":           "delete",
					"holding":          describeHolding(holding.Msg),
					"restored_by_sync": synced,
					"confirm_with":     "Repeat this call with dry_run=false to delete.",
				}
				if synced {
					plan["warning"] = syncRewriteWarning
				}
				return resultJSON(plan)
			}

			_, err = c.Portfolio.DeleteHolding(ctx, connect.NewRequest(&apiv1.DeleteHoldingRequest{Id: holdingID}))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			out := map[string]any{
				"deleted": describeHolding(holding.Msg),
			}
			if synced {
				out["warning"] = "Deleted, but this row came from sync: the next sync of its account " +
					"recreates it. Treat the deletion as temporary."
			}
			return resultJSON(out)
		},
	)
}

// planUnbind describes what removing target would do, including which holdings
// sit on the same chain as the binding.
func planUnbind(
	ctx context.Context,
	c *backend.Clients,
	asset *apiv1.Asset,
	target *apiv1.AssetExternalRef,
	remaining []*apiv1.AssetExternalRef,
) (map[string]any, error) {
	plan := map[string]any{
		"action":         "unbind",
		"asset":          assetLabel(asset),
		"binding":        refDescription(target),
		"remaining_refs": describeRefs(remaining),
		"effect": "The binding is removed from the catalogue. Holdings are NOT reassigned and no total " +
			"changes until the affected accounts are re-synced.",
		"confirm_with": "Repeat this call with dry_run=false to commit.",
	}

	chain, ok := chainOfRef(target)
	if !ok {
		return plan, nil
	}

	holdings, err := c.Portfolio.ListHoldings(ctx, connect.NewRequest(&apiv1.ListHoldingsRequest{
		AssetId: optString(asset.GetId()),
	}))
	if err != nil {
		// The chain breakdown is an enrichment; losing it must not block a plan
		// the caller can already act on.
		plan["holdings_note"] = fmt.Sprintf("Could not read holdings of this asset: %v", err)
		return plan, nil
	}

	var onChain []string
	for _, h := range holdings.Msg.GetHoldings() {
		if h.GetChain() != chain {
			continue
		}
		onChain = append(onChain, fmt.Sprintf("%s (%s on %s)",
			h.GetId(), scaledDecimal(h.GetAmount(), h.GetDecimals()), h.GetChain()))
	}

	if len(onChain) == 0 {
		plan["holdings_on_chain"] = fmt.Sprintf("No holdings of this asset sit on %s.", chain)
		return plan, nil
	}

	plan["holdings_on_chain"] = onChain
	plan["holdings_note"] = fmt.Sprintf(
		"%d holding(s) of this asset sit on %s. A holding records its chain, not the contract it came "+
			"from, so this is the set that COULD be affected — not proof that each came from this binding. "+
			"After the unbind and a re-sync, the ones that came from it move to their own asset and these "+
			"rows are zeroed; the rest are rewritten unchanged.",
		len(onChain), chain)
	return plan, nil
}

// planVerdict describes the verdict change against what is set now.
func planVerdict(ctx context.Context, c *backend.Clients, asset *apiv1.Asset, verdict string) (map[string]any, error) {
	current := asset.GetIdentityVerdict()
	if current == "" {
		current = "unknown"
	}

	plan := map[string]any{
		"action":          "set_verdict",
		"asset":           assetLabel(asset),
		"current_verdict": current,
		"current_source":  asset.GetVerdictSource(),
		"new_verdict":     verdict,
		"effect":          verdictEffect(current, verdict),
		"terminal": "A user verdict is terminal: rescoring will not overwrite it, and the automatic " +
			"signals stop being able to correct this asset.",
		"confirm_with": "Repeat this call with dry_run=false to commit.",
	}
	if score := asset.GetIdentityScore(); score > 0 {
		plan["current_score"] = score
	}
	if len(asset.GetIdentitySignals()) > 0 {
		plan["current_signals"] = asset.GetIdentitySignals()
	}

	holdings, err := c.Portfolio.ListHoldings(ctx, connect.NewRequest(&apiv1.ListHoldingsRequest{
		AssetId: optString(asset.GetId()),
	}))
	if err != nil {
		plan["holdings_note"] = fmt.Sprintf("Could not read holdings of this asset: %v", err)
		return plan, nil
	}

	var total, excluded int
	for _, h := range holdings.Msg.GetHoldings() {
		total++
		if h.GetExcluded() {
			excluded++
		}
	}
	plan["holdings_total"] = total
	plan["holdings_already_excluded"] = excluded

	if isQuarantineVerdict(verdict) {
		plan["holdings_effect"] = fmt.Sprintf(
			"All %d holding(s) of this asset leave every total, including any genuine position that was "+
				"merged into this asset by a wrong binding.", total)
	} else if isQuarantineVerdict(current) && excluded > 0 {
		plan["holdings_effect"] = fmt.Sprintf(
			"Withdrawing the quarantine does NOT release the %d holding(s) already excluded by it: the "+
				"excluded flag only rises. They stay out of every total until each is included by hand.",
			excluded)
	}
	return plan, nil
}

// verdictEffect states, in one sentence, what crossing the quarantine boundary
// in this direction does.
func verdictEffect(current, next string) string {
	switch {
	case isQuarantineVerdict(next) && !isQuarantineVerdict(current):
		return "Quarantines the asset: its holdings are excluded from every total."
	case !isQuarantineVerdict(next) && isQuarantineVerdict(current):
		return "Withdraws the quarantine. New holdings are no longer excluded; already-excluded rows are not released."
	case isQuarantineVerdict(next):
		return "Stays quarantined; the label changes."
	default:
		return "No quarantine either way: this records a judgement without changing any total."
	}
}

// verdictCommitNote is the after-the-fact half of verdictEffect.
func verdictCommitNote(previous, next string) string {
	if !isQuarantineVerdict(next) && isQuarantineVerdict(previous) {
		return "Quarantine withdrawn. Holdings already excluded by it were NOT released — the excluded " +
			"flag only rises. Check the asset's holdings and include the genuine ones by hand."
	}
	if isQuarantineVerdict(next) {
		return "Asset quarantined: its holdings are now out of every total."
	}
	return ""
}

// isQuarantineVerdict reports whether a verdict excludes holdings from totals.
// Mirrors the backend rule; the two must agree or the plan will describe an
// effect the commit does not produce.
func isQuarantineVerdict(v string) bool {
	return v == "scam" || v == "impersonation"
}

// isSettableVerdict guards the enum before the round trip: "unknown" is a state
// the heuristic can hold but not one a caller can set.
func isSettableVerdict(v string) bool {
	switch v {
	case "legit", "suspect", "scam", "impersonation":
		return true
	default:
		return false
	}
}

// partitionRefs splits refs into the one matching id and the rest.
func partitionRefs(refs []*apiv1.AssetExternalRef, id string) (*apiv1.AssetExternalRef, []*apiv1.AssetExternalRef) {
	var target *apiv1.AssetExternalRef
	remaining := make([]*apiv1.AssetExternalRef, 0, len(refs))
	for _, r := range refs {
		if r.GetId() == id {
			target = r
			continue
		}
		remaining = append(remaining, r)
	}
	return target, remaining
}

// chainOfRef returns the chain an "onchain:<chain>" ref names.
func chainOfRef(ref *apiv1.AssetExternalRef) (string, bool) {
	source := ref.GetSource()
	if !strings.HasPrefix(source, onchainRefPrefix) {
		return "", false
	}
	chain := strings.TrimPrefix(source, onchainRefPrefix)
	if chain == "" {
		return "", false
	}
	return chain, true
}

func refDescription(ref *apiv1.AssetExternalRef) string {
	return fmt.Sprintf("%s = %s (id %s, origin %s)",
		ref.GetSource(), ref.GetRef(), ref.GetId(), ref.GetOrigin())
}

func describeRefs(refs []*apiv1.AssetExternalRef) []string {
	if len(refs) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, refDescription(r))
	}
	return out
}

func assetLabel(a *apiv1.Asset) string {
	return fmt.Sprintf("%s (%s, id %s)", a.GetSymbol(), a.GetName(), a.GetId())
}

func describeHolding(h *apiv1.Holding) map[string]any {
	out := map[string]any{
		"id":         h.GetId(),
		"asset_id":   h.GetAssetId(),
		"account_id": h.GetAccountId(),
		"amount":     scaledDecimal(h.GetAmount(), h.GetDecimals()),
		"source":     h.GetSource().String(),
		"excluded":   h.GetExcluded(),
	}
	if h.GetChain() != "" {
		out["chain"] = h.GetChain()
	}
	if h.GetPortfolioId() != "" {
		out["portfolio_id"] = h.GetPortfolioId()
	}
	return out
}

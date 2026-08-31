package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/foxcool/greedy-eye-mcp/internal/backend"
	apiv1 "github.com/foxcool/greedy-eye/api/v1"
)

// protoJSONIn parses tool JSON arguments into proto request messages. Accepts
// both proto (snake_case) and JSON (camelCase) field names.
var protoJSONIn = protojson.UnmarshalOptions{DiscardUnknown: false}

// parseProtoArray unmarshals a JSON array string into proto messages built by mk.
func parseProtoArray[T proto.Message](raw string, mk func() T) ([]T, error) {
	var elems []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &elems); err != nil {
		return nil, fmt.Errorf("expected a JSON array: %w", err)
	}
	items := make([]T, 0, len(elems))
	for i, e := range elems {
		msg := mk()
		e, err := normalizeEnumFields(msg.ProtoReflect().Descriptor(), e)
		if err != nil {
			return nil, fmt.Errorf("item %d: %w", i, err)
		}
		if err := protoJSONIn.Unmarshal(e, msg); err != nil {
			return nil, fmt.Errorf("item %d: %w", i, err)
		}
		items = append(items, msg)
	}
	return items, nil
}

// registerMutatingTools wires write tools. Registered only when
// ENABLE_MUTATIONS=true: these create accounts, assets, holdings, and
// transaction history on the backend.
func registerMutatingTools(s *server.MCPServer, c *backend.Clients) {
	s.AddTool(
		mcp.NewTool("eye_create_manual_account",
			mcp.WithDescription("Create a manual account: no connector or credentials, positions are entered "+
				"by hand or imported via eye_import_positions. Use one account per real-world source "+
				"(a broker, a bank, a cold wallet)."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Account name, e.g. 'IB broker' or 'cold BTC'.")),
			mcp.WithString("description", mcp.Description("Optional free-form description.")),
			mcp.WithString("portfolio_id", mcp.Description("Portfolio UUID that imported holdings join by default.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := req.RequireString("name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			account := &apiv1.Account{
				Name:         name,
				Type:         apiv1.AccountType_ACCOUNT_TYPE_MANUAL,
				Capabilities: []string{"manual_positions"},
				Description:  optString(req.GetString("description", "")),
				PortfolioId:  optString(req.GetString("portfolio_id", "")),
			}
			resp, err := c.Portfolio.CreateAccount(ctx, connect.NewRequest(&apiv1.CreateAccountRequest{Account: account}))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return resultProto(resp.Msg)
		},
	)

	s.AddTool(
		mcp.NewTool("eye_find_or_create_asset",
			mcp.WithDescription("Resolve an asset by (symbol, market, type), creating it only when nothing "+
				"matches and dry_run is false. Find-first: always prefer an existing asset over creating "+
				"a duplicate. Market is implied only for cryptocurrency (crypto) and forex (forex); for "+
				"stock, bond, fund and commodity it is required, and it is validated before the lookup — "+
				"so it must be passed even when the asset already exists."),
			mcp.WithString("symbol", mcp.Required(), mcp.Description("Ticker symbol, e.g. BTC or AAPL.")),
			mcp.WithString("market", mcp.Description("Listing market: crypto, forex, nasdaq, moex, spbex, ... "+
				"Optional for cryptocurrency and forex, required for every other type.")),
			mcp.WithString("type", mcp.Description("Asset type; defaults to cryptocurrency. ETF is an alias for fund: "+
				"the exchange-traded part is carried by market."),
				mcp.Enum("", "cryptocurrency", "stock", "bond", "commodity", "forex", "fund", "etf")),
			mcp.WithString("name", mcp.Description("Asset name when created; defaults to the symbol.")),
			mcp.WithBoolean("dry_run", mcp.Description("When true, only reports whether the asset exists or would be created.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			symbol, err := req.RequireString("symbol")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			assetType, err := enumValue[apiv1.AssetType](
				apiv1.AssetType_ASSET_TYPE_UNSPECIFIED.Descriptor(), "type", req.GetString("type", ""))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			in := &apiv1.FindOrCreateAssetRequest{
				Symbol: symbol,
				Market: optString(req.GetString("market", "")),
				Type:   assetType,
				Name:   optString(req.GetString("name", "")),
				DryRun: req.GetBool("dry_run", false),
			}
			resp, err := c.MarketData.FindOrCreateAsset(ctx, connect.NewRequest(in))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return resultProto(resp.Msg)
		},
	)

	s.AddTool(
		mcp.NewTool("eye_import_positions",
			mcp.WithDescription("Batch-import positions into a MANUAL account. Simulation-first workflow: "+
				"ALWAYS call with dry_run=true first, show the returned plan (create/update/skip per item, "+
				"assets to be created) to the user, and only after explicit confirmation repeat the exact "+
				"same call with dry_run=false. One holding per (account, asset): existing holdings get their "+
				"amount refreshed, new ones are created with source=llm_import and the batch import_id. "+
				"With full_snapshot=true the batch is treated as the COMPLETE position list: holdings "+
				"absent from it are planned as DELETE and closed on commit (excluded holdings are never "+
				"touched; deletions are suppressed when any item fails). Use full_snapshot to reconcile "+
				"an account against a fresh export."),
			mcp.WithString("account_id", mcp.Required(), mcp.Description("Manual account UUID.")),
			mcp.WithString("positions", mcp.Required(), mcp.Description(
				`JSON array of position items: [{"symbol":"BTC","amount":"0.5"}, ...]. Fields: `+
					`symbol (or asset_id), amount (decimal string in asset units), `+
					`asset_type (cryptocurrency|stock|bond|commodity|forex|fund, etf aliases fund; default cryptocurrency), `+
					`market (listing venue: crypto, forex, nasdaq, moex, spbex, ...; optional only for cryptocurrency `+
					`and forex, REQUIRED for stock, bond, fund and commodity — including when the asset already exists), `+
					`name (optional, used if the asset is created), decimals (optional storage scale, default 8).`)),
			mcp.WithBoolean("dry_run", mcp.Description("Plan without writing. Defaults to true — pass false only to commit a confirmed plan.")),
			mcp.WithString("import_id", mcp.Description("Batch UUID; pass the same value on the commit call to keep one id for the whole import.")),
			mcp.WithBoolean("full_snapshot", mcp.Description("Reconcile mode: positions is the complete list for the account; absent holdings get closed. Confirm deletions with the user explicitly.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			accountID, err := req.RequireString("account_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			rawPositions, err := req.RequireString("positions")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			positions, err := parseProtoArray(rawPositions, func() *apiv1.ImportPositionItem { return &apiv1.ImportPositionItem{} })
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("positions: %v", err)), nil
			}
			in := &apiv1.ImportPositionsRequest{
				AccountId:    accountID,
				Positions:    positions,
				DryRun:       req.GetBool("dry_run", true), // default to the safe path
				ImportId:     optString(req.GetString("import_id", "")),
				FullSnapshot: req.GetBool("full_snapshot", false),
			}
			resp, err := c.Portfolio.ImportPositions(ctx, connect.NewRequest(in))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return resultProto(resp.Msg)
		},
	)

	s.AddTool(
		mcp.NewTool("eye_import_transactions",
			mcp.WithDescription("Batch-import transaction history into a MANUAL account. Same simulation-first "+
				"workflow as eye_import_positions: dry_run=true, confirm the plan, then commit. Duplicates are "+
				"skipped by external_id or the (type, asset, date, amount) tuple, so re-importing the same "+
				"export is safe. Never creates assets: unknown symbols fail per item."),
			mcp.WithString("account_id", mcp.Required(), mcp.Description("Manual account UUID.")),
			mcp.WithString("transactions", mcp.Required(), mcp.Description(
				`JSON array of transaction items: [{"type":"deposit","symbol":"BTC",`+
					`"external_id":"tx-1","data":{"date":"2026-07-01","amount":"0.1"}}, ...]. Fields: `+
					`type (required: trade|transfer|deposit|withdrawal|extended), `+
					`status (optional: pending|processing|completed|failed|cancelled, default completed), `+
					`symbol or asset_id (optional), external_id (optional dedup key), data (optional string map; `+
					`use date and amount keys to enable heuristic dedup).`)),
			mcp.WithBoolean("dry_run", mcp.Description("Plan without writing. Defaults to true — pass false only to commit a confirmed plan.")),
			mcp.WithString("import_id", mcp.Description("Batch UUID; pass the same value on the commit call to keep one id for the whole import.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			accountID, err := req.RequireString("account_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			rawTxs, err := req.RequireString("transactions")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			transactions, err := parseProtoArray(rawTxs, func() *apiv1.ImportTransactionItem { return &apiv1.ImportTransactionItem{} })
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("transactions: %v", err)), nil
			}
			in := &apiv1.ImportTransactionsRequest{
				AccountId:    accountID,
				Transactions: transactions,
				DryRun:       req.GetBool("dry_run", true), // default to the safe path
				ImportId:     optString(req.GetString("import_id", "")),
			}
			resp, err := c.Portfolio.ImportTransactions(ctx, connect.NewRequest(in))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return resultProto(resp.Msg)
		},
	)

	s.AddTool(
		mcp.NewTool("eye_reset_sweep_schedule",
			mcp.WithDescription("Forgive the price-sweep back-off accrued against named sources: their assets "+
				"become due again instead of waiting out a deferral earned under conditions that no longer hold. "+
				"Use when a source was unreachable or was being asked for assets it never covered, and its "+
				"schedule outlived the fix — the shape where a correct deploy produces no observable change for days. "+
				"ALWAYS call with dry_run=true first (the default), show the plan, and only repeat with "+
				"dry_run=false once the user confirms. This is an operator's statement, not an inference: it "+
				"withdraws the conclusion drawn from the attempt log without touching the log, and asserts "+
				"nothing about whether a price exists. The next sweep finds out. "+
				"Sources must be named — there is deliberately no 'all'."),
			mcp.WithArray("source_ids", mcp.Required(),
				mcp.Description("Sources to forgive, e.g. binance, cbr, coingecko, moex. Required: an empty list is rejected, not read as 'all'."),
				mcp.WithStringItems()),
			mcp.WithBoolean("dry_run", mcp.Description("Plan without writing. Defaults to true — pass false only to commit a confirmed plan.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			sourceIDs := req.GetStringSlice("source_ids", nil)
			if len(sourceIDs) == 0 {
				return mcp.NewToolResultError("source_ids is required: name the sources to reset"), nil
			}

			// The backend RPC has no dry_run — a reset is unconditional there. So the
			// plan is composed here from the schedule the reset would act on, which is
			// also the honest thing to show: how many rows are actually deferred is the
			// number that says whether the schedule was ever the problem.
			if req.GetBool("dry_run", true) { // default to the safe path
				sched, err := c.MarketData.GetSweepSchedule(ctx, connect.NewRequest(&apiv1.GetSweepScheduleRequest{}))
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				known := make(map[string]*apiv1.SourceSchedule, len(sched.Msg.GetSources()))
				for _, s := range sched.Msg.GetSources() {
					known[s.GetSourceId()] = s
				}
				plan := make([]map[string]any, 0, len(sourceIDs))
				for _, id := range sourceIDs {
					s, ok := known[id]
					if !ok {
						// Named but absent from the registry: the real call answers
						// NotFound, so say so now rather than promise a reset.
						plan = append(plan, map[string]any{"source_id": id, "error": "no such price source — the reset would fail with NotFound"})
						continue
					}
					plan = append(plan, map[string]any{
						"source_id":       id,
						"would_free":      s.GetDeferred(),
						"due_now":         s.GetDueNow(),
						"max_misses":      s.GetMaxMisses(),
						"latest_deferred": s.GetLatestDeferred().AsTime().Format(time.RFC3339),
					})
				}
				return resultJSON(map[string]any{
					"dry_run": true,
					"plan":    plan,
					"note": "would_free counts assets currently deferred; rows carrying misses but already due are " +
						"reset too, so the committed assets_freed can exceed it. Repeat with dry_run=false to commit.",
				})
			}

			in := &apiv1.ResetSweepScheduleRequest{SourceIds: sourceIDs}
			resp, err := c.MarketData.ResetSweepSchedule(ctx, connect.NewRequest(in))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return resultProtoWith(resp.Msg, map[string]any{
				"note": "Back-off withdrawn. Confirm with eye_get_sweep_schedule: deferred should fall and " +
					"due_now rise by roughly the same amount. Zero freed means the schedule was already clear.",
			})
		},
	)
}

package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

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
				"a duplicate. Market defaults by type (crypto/forex); required for stocks, bonds, funds."),
			mcp.WithString("symbol", mcp.Required(), mcp.Description("Ticker symbol, e.g. BTC or AAPL.")),
			mcp.WithString("market", mcp.Description("Listing market: crypto, forex, nasdaq, moex, ...")),
			mcp.WithString("type", mcp.Description("Asset type enum, e.g. ASSET_TYPE_STOCK. Defaults to ASSET_TYPE_CRYPTOCURRENCY.")),
			mcp.WithString("name", mcp.Description("Asset name when created; defaults to the symbol.")),
			mcp.WithBoolean("dry_run", mcp.Description("When true, only reports whether the asset exists or would be created.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			symbol, err := req.RequireString("symbol")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			in := &apiv1.FindOrCreateAssetRequest{
				Symbol: symbol,
				Market: optString(req.GetString("market", "")),
				Type:   apiv1.AssetType(apiv1.AssetType_value[req.GetString("type", "")]),
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
				"amount refreshed, new ones are created with source=llm_import and the batch import_id."),
			mcp.WithString("account_id", mcp.Required(), mcp.Description("Manual account UUID.")),
			mcp.WithString("positions", mcp.Required(), mcp.Description(
				`JSON array of position items: [{"symbol":"BTC","amount":"0.5"}, ...]. Fields: `+
					`symbol (or asset_id), amount (decimal string in asset units), `+
					`market (optional; crypto/forex/nasdaq/moex), asset_type (optional enum, default cryptocurrency), `+
					`name (optional, used if the asset is created), decimals (optional storage scale, default 8).`)),
			mcp.WithBoolean("dry_run", mcp.Description("Plan without writing. Defaults to true — pass false only to commit a confirmed plan.")),
			mcp.WithString("import_id", mcp.Description("Batch UUID; pass the same value on the commit call to keep one id for the whole import.")),
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
				AccountId: accountID,
				Positions: positions,
				DryRun:    req.GetBool("dry_run", true), // default to the safe path
				ImportId:  optString(req.GetString("import_id", "")),
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
				`JSON array of transaction items: [{"type":"TRANSACTION_TYPE_DEPOSIT","symbol":"BTC",`+
					`"external_id":"tx-1","data":{"date":"2026-07-01","amount":"0.1"}}, ...]. Fields: `+
					`type (TransactionType enum, required), status (optional, default completed), `+
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
}

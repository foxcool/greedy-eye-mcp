package mcpserver

import (
	"context"

	"connectrpc.com/connect"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/foxcool/greedy-eye-mcp/internal/backend"
	apiv1 "github.com/foxcool/greedy-eye/api/v1"
)

// registerPortfolioTools wires read-only PortfolioService tools.
func registerPortfolioTools(s *server.MCPServer, c *backend.Clients) {
	s.AddTool(
		mcp.NewTool("eye_list_portfolios",
			mcp.WithDescription("List portfolios, optionally filtered by user."),
			mcp.WithString("user_id", mcp.Description("Filter by owner user ID.")),
			mcp.WithNumber("page_size", mcp.Description("Max results per page."), mcp.Min(0)),
			mcp.WithString("page_token", mcp.Description("Pagination token.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			in := &apiv1.ListPortfoliosRequest{
				UserId:    optString(req.GetString("user_id", "")),
				PageSize:  optInt32(req.GetInt("page_size", 0)),
				PageToken: optString(req.GetString("page_token", "")),
			}
			resp, err := c.Portfolio.ListPortfolios(ctx, connect.NewRequest(in))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return resultProto(resp.Msg)
		},
	)

	s.AddTool(
		mcp.NewTool("eye_get_portfolio",
			mcp.WithDescription("Get a single portfolio by its ID."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Portfolio UUID.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id, err := req.RequireString("id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			resp, err := c.Portfolio.GetPortfolio(ctx, connect.NewRequest(&apiv1.GetPortfolioRequest{Id: id}))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return resultProto(resp.Msg)
		},
	)

	s.AddTool(
		mcp.NewTool("eye_list_accounts",
			mcp.WithDescription("List accounts (wallets, exchanges, manual sources), optionally filtered by type. "+
				"Secrets in account data are masked. Use this to find an existing account before creating one."),
			mcp.WithString("type", mcp.Description("Filter by account type, e.g. manual or wallet."),
				mcp.Enum("", "wallet", "exchange", "bank", "broker", "service", "manual")),
			mcp.WithNumber("page_size", mcp.Description("Max results per page."), mcp.Min(0)),
			mcp.WithString("page_token", mcp.Description("Pagination token.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			in := &apiv1.ListAccountsRequest{
				PageSize:  optInt32(req.GetInt("page_size", 0)),
				PageToken: optString(req.GetString("page_token", "")),
			}
			if raw := req.GetString("type", ""); raw != "" {
				t, err := enumValue[apiv1.AccountType](
					apiv1.AccountType_ACCOUNT_TYPE_UNSPECIFIED.Descriptor(), "type", raw)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				in.Type = &t
			}
			resp, err := c.Portfolio.ListAccounts(ctx, connect.NewRequest(in))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return resultProto(resp.Msg)
		},
	)

	s.AddTool(
		mcp.NewTool("eye_list_holdings",
			mcp.WithDescription("List holdings, optionally filtered by portfolio, account, or asset."),
			mcp.WithString("portfolio_id", mcp.Description("Filter by portfolio UUID.")),
			mcp.WithString("account_id", mcp.Description("Filter by account UUID.")),
			mcp.WithString("asset_id", mcp.Description("Filter by asset UUID.")),
			mcp.WithNumber("page_size", mcp.Description("Max results per page."), mcp.Min(0)),
			mcp.WithString("page_token", mcp.Description("Pagination token.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			in := &apiv1.ListHoldingsRequest{
				PortfolioId: optString(req.GetString("portfolio_id", "")),
				AccountId:   optString(req.GetString("account_id", "")),
				AssetId:     optString(req.GetString("asset_id", "")),
				PageSize:    optInt32(req.GetInt("page_size", 0)),
				PageToken:   optString(req.GetString("page_token", "")),
			}
			resp, err := c.Portfolio.ListHoldings(ctx, connect.NewRequest(in))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return resultProto(resp.Msg)
		},
	)

	s.AddTool(
		mcp.NewTool("eye_list_unpriced_holdings",
			mcp.WithDescription("Walk the positions a valuation leaves out of its total. "+
				"This is the worklist behind `coverage.unpriced`, which is only a capped sample: "+
				"the count there is exact, the list is a prefix. Here the whole tail is reachable, "+
				"a page at a time. "+
				"Each row is a piece of work, and the reason says which: NEVER_PRICED means every "+
				"source available was asked and none ever answered — usually an asset to bind or a "+
				"market to add, not a delisting verdict; THIN_MARKET means a quote exists but no "+
				"market behind it, which is a judgement its owner has to make; NO_CROSS_RATE means "+
				"the position IS priced, but in a base with no path to the display currency — one "+
				"exchange rate is missing, not coverage for the asset. "+
				"A PAGE IS NOT THE SET: report what this page holds, and say more remain whenever "+
				"`next_page_token` is non-empty. Pass it back to continue. "+
				"Positions excluded by a scam verdict are NOT here — they are out of the total by "+
				"decision rather than for want of a price, and are counted separately."),
			mcp.WithString("portfolio_id", mcp.Description(
				"Limit to one portfolio. Omit to walk every portfolio you own in a single pass.")),
			mcp.WithString("reason", mcp.Description(
				"Filter to one kind of gap: never_priced, thin_market, no_quote, or "+
					"no_cross_rate. Omit for all of them.")),
			mcp.WithNumber("page_size", mcp.Description("Rows per page. Defaults to 100."), mcp.Min(0)),
			mcp.WithString("page_token", mcp.Description(
				"Continue a walk: the `next_page_token` from the previous call.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			reason, err := enumValue[apiv1.UnpricedReason](
				apiv1.UnpricedReason(0).Descriptor(), "reason", req.GetString("reason", ""))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			in := &apiv1.ListUnpricedHoldingsRequest{
				PortfolioId: optString(req.GetString("portfolio_id", "")),
				PageSize:    optInt32(req.GetInt("page_size", 0)),
				PageToken:   optString(req.GetString("page_token", "")),
			}
			if reason != apiv1.UnpricedReason_UNPRICED_REASON_UNSPECIFIED {
				in.Reason = &reason
			}
			resp, err := c.Portfolio.ListUnpricedHoldings(ctx, connect.NewRequest(in))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			// The same discipline the coverage note keeps: a partial answer has to
			// say it is partial in words, next to the data. A model that reads
			// only the rows would otherwise report a page as the whole tail —
			// which is the failure this endpoint exists to end, reintroduced one
			// layer up.
			return resultProtoWith(resp.Msg, map[string]any{
				"page_note": unpricedPageNote(len(resp.Msg.GetHoldings()), resp.Msg.GetNextPageToken()),
			})
		},
	)

	s.AddTool(
		mcp.NewTool("eye_calculate_portfolio_value",
			mcp.WithDescription("Compute the total value of a portfolio in a quote currency. "+
				"Read-only: it values holdings, it does not change anything. "+
				"Adds a human-readable total alongside the raw scaled integer. "+
				"The total covers PRICED holdings only: positions with no usable quote stay out of "+
				"it and are reported in `coverage` / `coverage_note`. Quote the total together with "+
				"that coverage — a total presented alone reads as the whole portfolio. "+
				"`coverage_note` keeps two different doubts apart: holdings OUT of the total for want "+
				"of a price, and holdings IN it on a quote older than the instance's freshness policy. "+
				"Only the first kind is missing from the number; do not subtract the second."),
			mcp.WithString("portfolio_id", mcp.Required(), mcp.Description("Portfolio UUID.")),
			mcp.WithString("quote_asset_id", mcp.Description("Quote currency: asset UUID or ticker (e.g. USD). Defaults to USD.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			portfolioID, err := req.RequireString("portfolio_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			in := &apiv1.CalculatePortfolioValueRequest{
				PortfolioId:  portfolioID,
				QuoteAssetId: req.GetString("quote_asset_id", ""),
			}
			resp, err := c.Portfolio.CalculatePortfolioValue(ctx, connect.NewRequest(in))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			val := resp.Msg
			return resultProtoWith(val, map[string]any{
				"total_value_human": scaledDecimal(val.GetTotalValueAmount(), val.GetDecimals()),
				"coverage_note":     coverageNote(val.GetCoverage()),
			})
		},
	)

	s.AddTool(
		mcp.NewTool("eye_sync_account",
			mcp.WithDescription("Sync a wallet account's holdings from on-chain data (via Moralis). "+
				"A data-refresh action: it upserts assets/holdings for the account but moves no funds. "+
				"Only wallet-type accounts with a configured address can be synced."),
			mcp.WithString("account_id", mcp.Required(), mcp.Description("Account UUID to sync.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			accountID, err := req.RequireString("account_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			resp, err := c.Portfolio.SyncAccount(ctx, connect.NewRequest(&apiv1.SyncAccountRequest{AccountId: accountID}))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return resultProto(resp.Msg)
		},
	)
}

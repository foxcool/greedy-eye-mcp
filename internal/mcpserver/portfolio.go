package mcpserver

import (
	"context"
	"encoding/json"

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
		mcp.NewTool("eye_calculate_portfolio_value",
			mcp.WithDescription("Compute the total value of a portfolio in a quote currency. "+
				"Read-only: it values holdings, it does not change anything. "+
				"Adds a human-readable total alongside the raw scaled integer."),
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
			raw, mErr := protoJSON.Marshal(val)
			if mErr != nil {
				return resultProto(val)
			}
			var m map[string]any
			_ = json.Unmarshal(raw, &m)
			m["total_value_human"] = scaledDecimal(val.GetTotalValueAmount(), val.GetDecimals())
			return resultJSON(m)
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

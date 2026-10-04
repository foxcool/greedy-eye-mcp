package mcpserver

import (
	"context"

	"connectrpc.com/connect"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/foxcool/greedy-eye-mcp/internal/backend"
	apiv1 "github.com/foxcool/greedy-eye/api/v1"
)

// registerMarketDataTools wires read-only MarketDataService tools.
func registerMarketDataTools(s *server.MCPServer, c *backend.Clients) {
	s.AddTool(
		mcp.NewTool("eye_list_assets",
			mcp.WithDescription("List financial assets (crypto, stocks, etc.) tracked in greedy-eye. "+
				"Search with `query` rather than paging the catalogue: it is thousands of rows, most of them airdrop spam."),
			mcp.WithString("query", mcp.Description("Text search: symbol by prefix, name by substring (case-insensitive), "+
				"an exact asset id, or an exact bound external ref — a FIGI, a Solana mint in its exact case, an EVM 0x address in any case. At most 200 bytes.")),
			mcp.WithArray("ids", mcp.Description("Only these asset UUIDs (at most 1000)."), mcp.WithStringItems()),
			mcp.WithArray("tags", mcp.Description("Filter by tags (all must match)."), mcp.WithStringItems()),
			mcp.WithNumber("page_size", mcp.Description("Max results per page."), mcp.Min(0)),
			mcp.WithString("page_token", mcp.Description("Pagination token from a previous response.")),
			mcp.WithString("identity_verdict",
				mcp.Description("Filter by scam-filtering identity verdict: unknown | legit | suspect | scam | impersonation. Use scam/impersonation/suspect to review the quarantine."),
				mcp.Enum("unknown", "legit", "suspect", "scam", "impersonation")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			resp, err := c.MarketData.ListAssets(ctx, connect.NewRequest(listAssetsRequest(req)))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return resultProto(resp.Msg)
		},
	)

	s.AddTool(
		mcp.NewTool("eye_get_asset",
			mcp.WithDescription("Get a single asset by its ID."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Asset UUID.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id, err := req.RequireString("id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			resp, err := c.MarketData.GetAsset(ctx, connect.NewRequest(&apiv1.GetAssetRequest{Id: id}))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return resultProto(resp.Msg)
		},
	)

	s.AddTool(
		mcp.NewTool("eye_get_latest_price",
			mcp.WithDescription("Get the latest price of an asset quoted against a base asset. "+
				"Adds a human-readable price alongside the raw scaled integer."),
			mcp.WithString("asset_id", mcp.Required(), mcp.Description("Asset UUID being priced.")),
			mcp.WithString("base_asset_id", mcp.Required(), mcp.Description("Base/quote asset UUID.")),
			mcp.WithString("source_id", mcp.Description("Optional price source (e.g. coingecko, binance).")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			assetID, err := req.RequireString("asset_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			baseID, err := req.RequireString("base_asset_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			in := &apiv1.GetLatestPriceRequest{
				AssetId:     assetID,
				BaseAssetId: baseID,
				SourceId:    optString(req.GetString("source_id", "")),
			}
			resp, err := c.MarketData.GetLatestPrice(ctx, connect.NewRequest(in))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			price := resp.Msg
			return resultProtoWith(price, map[string]any{
				"last_human": scaledDecimal(price.GetLast(), price.GetDecimals()),
			})
		},
	)

	s.AddTool(
		mcp.NewTool("eye_list_price_history",
			mcp.WithDescription("List historical prices for an asset/base pair, optionally bounded by a time range."),
			mcp.WithString("asset_id", mcp.Required(), mcp.Description("Asset UUID.")),
			mcp.WithString("base_asset_id", mcp.Required(), mcp.Description("Base/quote asset UUID.")),
			mcp.WithString("from", mcp.Description("Start time, RFC3339 (e.g. 2026-01-01T00:00:00Z).")),
			mcp.WithString("to", mcp.Description("End time, RFC3339.")),
			mcp.WithString("source_id", mcp.Description("Optional price source.")),
			mcp.WithNumber("page_size", mcp.Description("Max results per page."), mcp.Min(0)),
			mcp.WithString("page_token", mcp.Description("Pagination token.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			assetID, err := req.RequireString("asset_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			baseID, err := req.RequireString("base_asset_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			in := &apiv1.ListPriceHistoryRequest{
				AssetId:     assetID,
				BaseAssetId: baseID,
				SourceId:    optString(req.GetString("source_id", "")),
				PageSize:    optInt32(req.GetInt("page_size", 0)),
				PageToken:   optString(req.GetString("page_token", "")),
			}
			if v := req.GetString("from", ""); v != "" {
				ts, err := parseTimestamp(v)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				in.From = ts
			}
			if v := req.GetString("to", ""); v != "" {
				ts, err := parseTimestamp(v)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				in.To = ts
			}
			resp, err := c.MarketData.ListPriceHistory(ctx, connect.NewRequest(in))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return resultProto(resp.Msg)
		},
	)

	s.AddTool(
		mcp.NewTool("eye_fetch_external_prices",
			mcp.WithDescription("Refresh prices from external sources (CoinGecko, Binance) and store them. "+
				"A data-refresh action: it writes prices but moves no funds. "+
				"Defaults to all assets and all configured sources."),
			mcp.WithArray("asset_ids", mcp.Description("Limit to these asset UUIDs (default: all)."), mcp.WithStringItems()),
			mcp.WithArray("source_ids", mcp.Description("Limit to these sources, e.g. coingecko, binance (default: all)."), mcp.WithStringItems()),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			in := &apiv1.FetchExternalPricesRequest{
				AssetIds:  req.GetStringSlice("asset_ids", nil),
				SourceIds: req.GetStringSlice("source_ids", nil),
			}
			resp, err := c.MarketData.FetchExternalPrices(ctx, connect.NewRequest(in))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return resultProto(resp.Msg)
		},
	)

	s.AddTool(
		mcp.NewTool("eye_get_sweep_schedule",
			mcp.WithDescription("Show the price sweep's queue per source: how many assets are due now, how many "+
				"are deferred by back-off, and how deep that back-off runs (max_misses). "+
				"Answers why prices are not moving when a fetch reports nothing fetched — "+
				"'everything is current' and 'the whole catalogue is postponed' look identical otherwise. "+
				"A latest_deferred a week out is the signature of assets that hit the back-off ceiling. "+
				"Read-only; eye_reset_sweep_schedule is what withdraws a deferral."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			resp, err := c.MarketData.GetSweepSchedule(ctx, connect.NewRequest(&apiv1.GetSweepScheduleRequest{}))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return resultProto(resp.Msg)
		},
	)
}

// listAssetsRequest maps the tool's arguments onto the RPC. An empty query or
// id list is left out rather than sent empty: either way the backend applies
// no filter, and leaving it out keeps the request saying only what was asked.
func listAssetsRequest(req mcp.CallToolRequest) *apiv1.ListAssetsRequest {
	return &apiv1.ListAssetsRequest{
		Query:           optString(req.GetString("query", "")),
		Ids:             req.GetStringSlice("ids", nil),
		Tags:            req.GetStringSlice("tags", nil),
		PageSize:        optInt32(req.GetInt("page_size", 0)),
		PageToken:       optString(req.GetString("page_token", "")),
		IdentityVerdict: optString(req.GetString("identity_verdict", "")),
	}
}

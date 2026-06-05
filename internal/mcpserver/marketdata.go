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

// registerMarketDataTools wires read-only MarketDataService tools.
func registerMarketDataTools(s *server.MCPServer, c *backend.Clients) {
	s.AddTool(
		mcp.NewTool("eye_list_assets",
			mcp.WithDescription("List financial assets (crypto, stocks, etc.) tracked in greedy-eye."),
			mcp.WithArray("tags", mcp.Description("Filter by tags (all must match)."), mcp.WithStringItems()),
			mcp.WithNumber("page_size", mcp.Description("Max results per page."), mcp.Min(0)),
			mcp.WithString("page_token", mcp.Description("Pagination token from a previous response.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			in := &apiv1.ListAssetsRequest{
				Tags:      req.GetStringSlice("tags", nil),
				PageSize:  optInt32(req.GetInt("page_size", 0)),
				PageToken: optString(req.GetString("page_token", "")),
			}
			resp, err := c.MarketData.ListAssets(ctx, connect.NewRequest(in))
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
			raw, mErr := protoJSON.Marshal(price)
			if mErr != nil {
				return resultProto(price)
			}
			var m map[string]any
			_ = json.Unmarshal(raw, &m)
			m["last_human"] = scaledDecimal(price.GetLast(), price.GetDecimals())
			return resultJSON(m)
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
}

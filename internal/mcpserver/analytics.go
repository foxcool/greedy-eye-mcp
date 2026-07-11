package mcpserver

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/foxcool/greedy-eye-mcp/internal/backend"
	apiv1 "github.com/foxcool/greedy-eye/api/v1"
)

// registerAnalyticsTools wires read-only AnalyticsService tools.
func registerAnalyticsTools(s *server.MCPServer, c *backend.Clients) {
	s.AddTool(
		mcp.NewTool("eye_get_heatmap",
			mcp.WithDescription("Portfolio heatmap: treemap nodes where size = holding value in the "+
				"quote asset and color_value = price change % over the window. Group nodes (empty "+
				"parent_id, no asset_id) aggregate their children."),
			mcp.WithString("portfolio_id", mcp.Required(), mcp.Description("Portfolio UUID (heatmap scope).")),
			mcp.WithString("group_by", mcp.Description("Grouping axis: 'account' or empty for a flat map."),
				mcp.Enum("", "account")),
			mcp.WithString("window", mcp.Description("Change window: 24h (default), 7d, or 30d."),
				mcp.Enum("", "24h", "7d", "30d")),
			mcp.WithString("quote_asset_id", mcp.Description("Asset the values are quoted in; defaults to USD.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			portfolioID, err := req.RequireString("portfolio_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			in := &apiv1.GetHeatmapRequest{
				Scope:        apiv1.HeatmapScope_HEATMAP_SCOPE_PORTFOLIO,
				ScopeId:      portfolioID,
				QuoteAssetId: req.GetString("quote_asset_id", ""),
			}

			switch groupBy := req.GetString("group_by", ""); groupBy {
			case "":
			case "account":
				in.GroupBy = apiv1.HeatmapGroupBy_HEATMAP_GROUP_BY_ACCOUNT
			default:
				return mcp.NewToolResultError(fmt.Sprintf("unsupported group_by %q", groupBy)), nil
			}

			switch window := req.GetString("window", ""); window {
			case "", "24h":
				in.Window = apiv1.HeatmapWindow_HEATMAP_WINDOW_24H
			case "7d":
				in.Window = apiv1.HeatmapWindow_HEATMAP_WINDOW_7D
			case "30d":
				in.Window = apiv1.HeatmapWindow_HEATMAP_WINDOW_30D
			default:
				return mcp.NewToolResultError(fmt.Sprintf("unsupported window %q", window)), nil
			}

			resp, err := c.Analytics.GetHeatmap(ctx, connect.NewRequest(in))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return resultProto(resp.Msg)
		},
	)
}

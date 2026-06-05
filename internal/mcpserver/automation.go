package mcpserver

import (
	"context"

	"connectrpc.com/connect"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/foxcool/greedy-eye-mcp/internal/backend"
	apiv1 "github.com/foxcool/greedy-eye/api/v1"
)

// registerAutomationTools wires read-only / non-mutating AutomationService tools.
// Execute/enable/disable/create/delete are intentionally NOT exposed here: an
// LLM-facing surface must not be able to trigger trades, withdrawals, or rule
// state changes. Add those deliberately behind ENABLE_MUTATIONS with care.
func registerAutomationTools(s *server.MCPServer, c *backend.Clients) {
	s.AddTool(
		mcp.NewTool("eye_list_rules",
			mcp.WithDescription("List automation rules, with optional filters."),
			mcp.WithString("user_id", mcp.Description("Filter by user ID.")),
			mcp.WithString("portfolio_id", mcp.Description("Filter by portfolio UUID.")),
			mcp.WithString("rule_type", mcp.Description("Filter by rule type (e.g. dca, stop_loss).")),
			mcp.WithString("status", mcp.Description("Filter by status enum, e.g. RULE_STATUS_ACTIVE."),
				mcp.Enum(
					"RULE_STATUS_UNKNOWN",
					"RULE_STATUS_ACTIVE",
					"RULE_STATUS_PAUSED",
					"RULE_STATUS_DISABLED",
					"RULE_STATUS_ERROR",
				),
			),
			mcp.WithNumber("page_size", mcp.Description("Max results per page."), mcp.Min(0)),
			mcp.WithString("page_token", mcp.Description("Pagination token.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			in := &apiv1.ListRulesRequest{
				UserId:      optString(req.GetString("user_id", "")),
				PortfolioId: optString(req.GetString("portfolio_id", "")),
				RuleType:    optString(req.GetString("rule_type", "")),
				PageSize:    optInt32(req.GetInt("page_size", 0)),
				PageToken:   optString(req.GetString("page_token", "")),
			}
			if v := req.GetString("status", ""); v != "" {
				code, ok := apiv1.RuleStatus_value[v]
				if !ok {
					return mcp.NewToolResultError("invalid status; expected e.g. RULE_STATUS_ACTIVE"), nil
				}
				st := apiv1.RuleStatus(code)
				in.Status = &st
			}
			resp, err := c.Automation.ListRules(ctx, connect.NewRequest(in))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return resultProto(resp.Msg)
		},
	)

	s.AddTool(
		mcp.NewTool("eye_get_rule",
			mcp.WithDescription("Get a single automation rule by its ID."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Rule UUID.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id, err := req.RequireString("id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			resp, err := c.Automation.GetRule(ctx, connect.NewRequest(&apiv1.GetRuleRequest{Id: id}))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return resultProto(resp.Msg)
		},
	)

	s.AddTool(
		mcp.NewTool("eye_simulate_rule",
			mcp.WithDescription("Simulate (dry-run) what a rule would do, without executing it. "+
				"Safe: produces an estimate of trades/withdrawals/costs only."),
			mcp.WithString("rule_id", mcp.Required(), mcp.Description("Rule UUID to simulate.")),
			mcp.WithString("simulate_at", mcp.Description("Point in time to simulate at, RFC3339. Defaults to now.")),
			mcp.WithBoolean("include_costs", mcp.Description("Include estimated fees/costs in the result.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ruleID, err := req.RequireString("rule_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			in := &apiv1.SimulateRuleRequest{
				RuleId:       ruleID,
				IncludeCosts: req.GetBool("include_costs", false),
			}
			if v := req.GetString("simulate_at", ""); v != "" {
				ts, err := parseTimestamp(v)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				in.SimulateAt = ts
			}
			resp, err := c.Automation.SimulateRule(ctx, connect.NewRequest(in))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return resultProto(resp.Msg)
		},
	)
}

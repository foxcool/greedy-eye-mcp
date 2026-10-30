package mcpserver

import "github.com/mark3labs/mcp-go/mcp"

// Tool annotations. mcp.NewTool defaults every tool to readOnlyHint=false,
// destructiveHint=true, so a tool registered without one of these options tells
// clients that a plain read may destroy data. Every tool must pass exactly one;
// TestToolAnnotations holds the classification and fails on an unclassified tool.
//
// The hints describe the commit path: a write that defaults to dry_run=true is
// still a write, because the same tool commits when dry_run=false.

// readOnlyTool marks a tool that only reads backend state.
func readOnlyTool() mcp.ToolOption {
	return func(t *mcp.Tool) {
		mcp.WithReadOnlyHintAnnotation(true)(t)
		mcp.WithDestructiveHintAnnotation(false)(t)
		mcp.WithIdempotentHintAnnotation(true)(t)
	}
}

// writeTool marks a tool that changes backend state. destructive means it can
// delete or overwrite existing data rather than only add to it; idempotent means
// repeating the call with the same arguments has no further effect.
func writeTool(destructive, idempotent bool) mcp.ToolOption {
	return func(t *mcp.Tool) {
		mcp.WithReadOnlyHintAnnotation(false)(t)
		mcp.WithDestructiveHintAnnotation(destructive)(t)
		mcp.WithIdempotentHintAnnotation(idempotent)(t)
	}
}

package mcpserver

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// protoJSON renders a protobuf message as readable JSON using the proto field
// names (snake_case), so the model sees the same shape as the .proto schema.
var protoJSON = protojson.MarshalOptions{
	UseProtoNames:   true,
	EmitUnpopulated: false,
	Indent:          "  ",
}

// resultProto marshals a protobuf response message into a tool text result.
func resultProto(msg proto.Message) (*mcp.CallToolResult, error) {
	b, err := protoJSON.Marshal(msg)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode response: %v", err)), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}

// resultJSON marshals an arbitrary value (typically an enriched map) into a result.
func resultJSON(v any) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode response: %v", err)), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}

// scaledDecimal converts a raw integer string scaled by `decimals` into a
// human-readable decimal string. greedy-eye stores balances and prices this way
// because on-chain uint256 values overflow int64. Returns the input unchanged
// if it is not a plain integer.
func scaledDecimal(raw string, decimals uint32) string {
	if raw == "" {
		return ""
	}
	n, ok := new(big.Int).SetString(raw, 10)
	if !ok {
		return raw
	}
	if decimals == 0 {
		return n.String()
	}

	neg := n.Sign() < 0
	digits := new(big.Int).Abs(n).String()
	d := int(decimals)
	if len(digits) <= d {
		digits = strings.Repeat("0", d-len(digits)+1) + digits
	}

	intPart := digits[:len(digits)-d]
	fracPart := strings.TrimRight(digits[len(digits)-d:], "0")

	out := intPart
	if fracPart != "" {
		out += "." + fracPart
	}
	if neg {
		out = "-" + out
	}
	return out
}

// parseTimestamp parses an RFC3339 string into a protobuf Timestamp.
func parseTimestamp(s string) (*timestamppb.Timestamp, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, fmt.Errorf("expected RFC3339 timestamp, got %q: %w", s, err)
	}
	return timestamppb.New(t), nil
}

// optString returns a pointer to s, or nil if s is empty. Useful for proto3
// `optional string` request fields where empty means "unset".
func optString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// optInt32 returns a pointer to int32(i), or nil if i == 0. The value is clamped
// to the int32 range so an out-of-range page size can never silently overflow
// (gosec G115).
func optInt32(i int) *int32 {
	if i == 0 {
		return nil
	}
	if i > math.MaxInt32 {
		i = math.MaxInt32
	} else if i < math.MinInt32 {
		i = math.MinInt32
	}
	v := int32(i)
	return &v
}

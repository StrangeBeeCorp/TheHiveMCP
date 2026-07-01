package utils

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// NewToolResultJSONUnescaped builds an MCP tool result without escaping
// non-ASCII characters, so Unicode stays literal, not \uXXXX-escaped.
func NewToolResultJSONUnescaped(data interface{}) *mcp.CallToolResult {
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)

	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(data); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode data to JSON: %v", err))
	}

	// Drop the trailing newline encoder.Encode appends.
	jsonBytes := bytes.TrimSuffix(buffer.Bytes(), []byte("\n"))

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: string(jsonBytes),
			},
		},
		StructuredContent: data,
	}
}

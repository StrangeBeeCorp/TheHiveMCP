package utils

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// NewToolResultJSONUnescaped creates a new MCP tool result with proper UTF-8 encoding
// without escaping non-ASCII characters. This fixes issues where Unicode characters
// like emojis and accented characters get escaped (e.g., ✅ becomes \u2705).
func NewToolResultJSONUnescaped(data interface{}) *mcp.CallToolResult {
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)

	// Prevent HTML escaping and allow proper UTF-8 encoding
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(data); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode data to JSON: %v", err))
	}

	// Remove the trailing newline that encoder.Encode adds
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

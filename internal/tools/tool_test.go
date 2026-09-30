package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTool is a BaseTool with a caller-chosen definition.
type fakeTool struct {
	definition mcp.Tool
}

func (f fakeTool) Name() string         { return f.definition.Name }
func (f fakeTool) Definition() mcp.Tool { return f.definition }

func (f fakeTool) Handler() server.ToolHandlerFunc {
	return func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText(""), nil
	}
}

// registeredInputSchema registers definition through a Registry and returns the
// input schema the server then advertises for it.
func registeredInputSchema(t *testing.T, definition mcp.Tool) json.RawMessage {
	t.Helper()

	registry := NewRegistry()
	registry.Register(fakeTool{definition: definition})

	mcpServer := server.NewMCPServer("test", "0.0.0")
	registry.RegisterAll(mcpServer)

	registered := mcpServer.GetTool(definition.Name)
	require.NotNil(t, registered, "tool %q was not registered", definition.Name)

	return registered.Tool.RawInputSchema
}

// registeredProperty returns, re-encoded, one property of the input schema the
// server advertises for definition.
func registeredProperty(t *testing.T, definition mcp.Tool, name string) string {
	t.Helper()

	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(registeredInputSchema(t, definition), &schema),
		"tool %q has an undecodable input schema", definition.Name)
	require.Contains(t, schema.Properties, name, "tool %q advertises no property %q", definition.Name, name)

	return string(schema.Properties[name])
}

// A tool that bypasses WithInputSchemaConstraints, as get-resource does with a
// bare mcp.WithInputSchema, must still reach the wire without a null union:
// registration is the one step every tool goes through.
func TestRegisterAll_DropsNullFromABareInferredSchema(t *testing.T) {
	t.Parallel()

	type params struct {
		IDs []string `json:"ids,omitempty"`
	}

	bare := mcp.NewTool("bare", mcp.WithInputSchema[params]())

	assert.JSONEq(t, `{"type": "array", "items": {"type": "string"}}`, registeredProperty(t, bare, "ids"),
		"[]string field `ids`, inferred as [\"null\", \"array\"]")
}

// Nested collections put the inferred union below items and
// additionalProperties, where a narrowing that stopped at the property would
// leave it.
func TestRegisterAll_DropsNullBelowItemsAndAdditionalProperties(t *testing.T) {
	t.Parallel()

	type params struct {
		Matrix [][]string          `json:"matrix,omitempty"`
		Index  map[string][]string `json:"index,omitempty"`
	}

	nested := mcp.NewTool("nested", mcp.WithInputSchema[params]())

	assert.JSONEq(t, `{"type": "array", "items": {"type": "array", "items": {"type": "string"}}}`,
		registeredProperty(t, nested, "matrix"), "[][]string field `matrix`")
	assert.JSONEq(t, `{"type": "object", "additionalProperties": {"type": "array", "items": {"type": "string"}}}`,
		registeredProperty(t, nested, "index"), "map[string][]string field `index`")
}

// A schema without a top-level properties object, which applyConstraints
// refuses, must be narrowed all the same.
func TestRegisterAll_DropsNullFromASchemaWithoutProperties(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"type": "object", "$defs": {"ids": {"type": ["null", "array"]}}}`)

	assert.JSONEq(t, `{"type": "object", "$defs": {"ids": {"type": "array"}}}`,
		string(registeredInputSchema(t, mcp.NewTool("no-properties", mcp.WithRawInputSchema(raw)))))
}

package tools

import (
	"context"
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

// Registration advertises the definition as it is. The schema is settled when
// the tool is built (WithInputSchema, WithInputSchemaConstraints), so what a
// test reads from Definition() is what a client receives; a tool built without
// those options is caught by TestToolSchemas_InputTypesAdmitNoNull instead of
// being silently rewritten here.
func TestRegisterAll_AdvertisesTheDefinitionAsItIs(t *testing.T) {
	t.Parallel()

	raw := `{"type": "object", "properties": {"ids": {"type": ["null", "array"]}}}`
	definition := mcp.NewTool("as-is", mcp.WithRawInputSchema([]byte(raw)))

	registry := NewRegistry()
	registry.Register(fakeTool{definition: definition})

	mcpServer := server.NewMCPServer("test", "0.0.0")
	registry.RegisterAll(mcpServer)

	registered := mcpServer.GetTool(definition.Name)
	require.NotNil(t, registered, "tool %q was not registered", definition.Name)
	assert.Equal(t, raw, string(registered.Tool.RawInputSchema), `definition with an ["null", "array"] property`)
}

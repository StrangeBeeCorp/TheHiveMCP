package bootstrap

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/auth"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
)

func TestAuthenticationExecutionOrder(t *testing.T) {
	t.Run("middleware runs after context function and catches auth error", func(t *testing.T) {
		options := &types.TheHiveMcpDefaultOptions{
			TheHiveURL:          "https://invalid-thehive-server-that-does-not-exist.com",
			TheHiveAPIKey:       "test-key",
			TheHiveOrganisation: "test-org",
		}

		mcpServer := server.NewMCPServer("test", "1.0.0",
			server.WithToolHandlerMiddleware(auth.AuthenticationMiddleware()),
		)

		testToolCalled := false
		mcpServer.AddTool(mcp.NewTool("test-tool"), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			testToolCalled = true
			return &mcp.CallToolResult{}, nil
		})

		httpServer := server.NewStreamableHTTPServer(mcpServer,
			server.WithHTTPContextFunc(GetHTTPAuthContextFunc(options)),
		)

		reqBody := strings.NewReader(`{
			"jsonrpc": "2.0",
			"id": 1,
			"method": "tools/call",
			"params": {
				"name": "test-tool",
				"arguments": {}
			}
		}`)

		req := httptest.NewRequest("POST", "/", reqBody)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(string(types.HeaderKeyTheHiveURL), options.TheHiveURL)
		req.Header.Set(string(types.HeaderKeyTheHiveAPIKey), options.TheHiveAPIKey)
		req.Header.Set(string(types.HeaderKeyTheHiveOrganisation), options.TheHiveOrganisation)

		w := httptest.NewRecorder()

		// Order under test: context func fails validation and stores the auth
		// error, then middleware catches it and blocks the tool.
		httpServer.ServeHTTP(w, req)

		response := w.Body.String()
		t.Logf("Response: %s", response)

		assert.False(t, testToolCalled, "Tool should not be called when authentication fails")
		assert.Contains(t, response, "Invalid session ID", "Response should contain authentication error")
	})
}

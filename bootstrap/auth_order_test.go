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
		// Create options that will fail TheHive validation (invalid URL)
		options := &types.TheHiveMcpDefaultOptions{
			TheHiveURL:          "https://invalid-thehive-server-that-does-not-exist.com", // Will fail validation
			TheHiveAPIKey:       "test-key",
			TheHiveOrganisation: "test-org",
		}

		// Create a simple test tool
		mcpServer := server.NewMCPServer("test", "1.0.0",
			server.WithToolHandlerMiddleware(auth.AuthenticationMiddleware()),
		)

		testToolCalled := false
		mcpServer.AddTool(mcp.NewTool("test-tool"), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			testToolCalled = true
			return &mcp.CallToolResult{}, nil
		})

		// Create HTTP server with context function
		httpServer := server.NewStreamableHTTPServer(mcpServer,
			server.WithHTTPContextFunc(GetHTTPAuthContextFunc(options)),
		)

		// Create test HTTP request for tool call
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

		// This should trigger:
		// 1. Context function (which will fail validation and store auth error)
		// 2. Middleware (which should catch the auth error and return it)
		// 3. Tool should NOT be called
		httpServer.ServeHTTP(w, req)

		// Debug: print actual response
		response := w.Body.String()
		t.Logf("Response: %s", response)

		// Verify that:
		// 1. The test tool was NOT called (middleware blocked it)
		assert.False(t, testToolCalled, "Tool should not be called when authentication fails")

		// 2. We got an authentication error response
		assert.Contains(t, response, "Invalid session ID", "Response should contain authentication error")
	})
}

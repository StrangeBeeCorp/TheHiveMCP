package bootstrap

import (
	"context"
	"fmt"
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/auth"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
)

func TestAuthenticationMiddleware(t *testing.T) {
	middleware := auth.AuthenticationMiddleware()

	t.Run("allows request when no auth error", func(t *testing.T) {
		ctx := context.Background()
		called := false

		handler := middleware(func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			called = true
			return &mcp.CallToolResult{}, nil
		})

		_, err := handler(ctx, mcp.CallToolRequest{})

		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("blocks request when auth error exists", func(t *testing.T) {
		authError := fmt.Errorf("TheHive authentication failed: invalid credentials")
		ctx := context.WithValue(context.Background(), types.AuthErrorCtxKey, authError)
		called := false

		handler := middleware(func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			called = true
			return &mcp.CallToolResult{}, nil
		})

		_, err := handler(ctx, mcp.CallToolRequest{})

		assert.Error(t, err)
		assert.Equal(t, authError, err)
		assert.False(t, called)
	})
}

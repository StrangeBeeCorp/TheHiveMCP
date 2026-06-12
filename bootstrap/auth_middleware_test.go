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

	t.Run("allows request when authentication was validated", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), types.AuthValidatedCtxKey, true)
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

	t.Run("blocks request when validation marker is absent (fail closed)", func(t *testing.T) {
		// No auth error and no validation marker: the request must still be
		// denied, never silently allowed (RandoriSec 5.5)
		ctx := context.Background()
		called := false

		handler := middleware(func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			called = true
			return &mcp.CallToolResult{}, nil
		})

		_, err := handler(ctx, mcp.CallToolRequest{})

		assert.ErrorIs(t, err, auth.ErrAuthenticationNotValidated)
		assert.False(t, called)
	})

	t.Run("auth error takes precedence over missing marker", func(t *testing.T) {
		authError := fmt.Errorf("TheHive authentication failed: URL is not in the allowlist")
		ctx := context.WithValue(context.Background(), types.AuthErrorCtxKey, authError)

		handler := middleware(func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{}, nil
		})

		_, err := handler(ctx, mcp.CallToolRequest{})

		assert.Equal(t, authError, err)
	})
}

func TestResourceAuthenticationMiddleware(t *testing.T) {
	middleware := auth.ResourceAuthenticationMiddleware()

	t.Run("allows request when authentication was validated", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), types.AuthValidatedCtxKey, true)
		called := false

		handler := middleware(func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			called = true
			return nil, nil
		})

		_, err := handler(ctx, mcp.ReadResourceRequest{})

		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("blocks request when validation marker is absent", func(t *testing.T) {
		called := false

		handler := middleware(func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			called = true
			return nil, nil
		})

		_, err := handler(context.Background(), mcp.ReadResourceRequest{})

		assert.ErrorIs(t, err, auth.ErrAuthenticationNotValidated)
		assert.False(t, called)
	})
}

func TestAuthenticatedPromptHandlerFunc(t *testing.T) {
	t.Run("allows request when authentication was validated", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), types.AuthValidatedCtxKey, true)
		called := false

		handler := auth.AuthenticatedPromptHandlerFunc(func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			called = true
			return &mcp.GetPromptResult{}, nil
		})

		_, err := handler(ctx, mcp.GetPromptRequest{})

		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("blocks request when validation marker is absent", func(t *testing.T) {
		called := false

		handler := auth.AuthenticatedPromptHandlerFunc(func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			called = true
			return &mcp.GetPromptResult{}, nil
		})

		_, err := handler(context.Background(), mcp.GetPromptRequest{})

		assert.ErrorIs(t, err, auth.ErrAuthenticationNotValidated)
		assert.False(t, called)
	})
}

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

// invokeThroughAuth runs a no-op handler wrapped by the given auth layer and
// reports whether the handler was reached and which error came back.
type invokeThroughAuth func(ctx context.Context) (called bool, err error)

func authWrappers() map[string]invokeThroughAuth {
	return map[string]invokeThroughAuth{
		"tool middleware": func(ctx context.Context) (bool, error) {
			called := false
			handler := auth.AuthenticationMiddleware()(func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				called = true
				return &mcp.CallToolResult{}, nil
			})
			_, err := handler(ctx, mcp.CallToolRequest{})
			return called, err
		},
		"resource middleware": func(ctx context.Context) (bool, error) {
			called := false
			handler := auth.ResourceAuthenticationMiddleware()(func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				called = true
				return nil, nil
			})
			_, err := handler(ctx, mcp.ReadResourceRequest{})
			return called, err
		},
		"prompt handler": func(ctx context.Context) (bool, error) {
			called := false
			handler := auth.AuthenticatedPromptHandlerFunc(func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				called = true
				return &mcp.GetPromptResult{}, nil
			})
			_, err := handler(ctx, mcp.GetPromptRequest{})
			return called, err
		},
	}
}

func TestAuthenticationChecks(t *testing.T) {
	authError := fmt.Errorf("TheHive authentication failed: invalid credentials")

	scenarios := []struct {
		name string
		// ctx is the per-scenario request context, not a stored/long-lived
		// field — the S8242 "pass context as a parameter" rule does not apply
		// to a test-case table. NOSONAR
		ctx         context.Context
		expectCall  bool
		expectedErr error
	}{
		{
			name:       "allows request when authentication was validated",
			ctx:        context.WithValue(context.Background(), types.AuthValidatedCtxKey, true),
			expectCall: true,
		},
		{
			name:        "blocks request when auth error exists",
			ctx:         context.WithValue(context.Background(), types.AuthErrorCtxKey, authError),
			expectedErr: authError,
		},
		{
			// No auth error and no validation marker: the request must still
			// be denied, never silently allowed (RandoriSec 5.5)
			name:        "blocks request when validation marker is absent (fail closed)",
			ctx:         context.Background(),
			expectedErr: auth.ErrAuthenticationNotValidated,
		},
		{
			name:        "auth error takes precedence over missing marker",
			ctx:         context.WithValue(context.Background(), types.AuthErrorCtxKey, authError),
			expectedErr: authError,
		},
	}

	for wrapperName, invoke := range authWrappers() {
		for _, scenario := range scenarios {
			t.Run(wrapperName+" "+scenario.name, func(t *testing.T) {
				called, err := invoke(scenario.ctx)

				assert.Equal(t, scenario.expectCall, called)
				if scenario.expectedErr != nil {
					assert.ErrorIs(t, err, scenario.expectedErr)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	}
}

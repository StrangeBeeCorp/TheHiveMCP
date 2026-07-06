package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/auth"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// invokeThroughAuth runs a no-op handler wrapped by the given auth layer and
// reports whether the handler was reached and which error came back.
type invokeThroughAuth func(ctx context.Context) (called bool, err error)

func authWrappers() map[string]invokeThroughAuth {
	return map[string]invokeThroughAuth{
		"tool middleware": func(ctx context.Context) (bool, error) {
			called := false
			handler := auth.AuthenticationMiddleware()(func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				called = true
				return &mcp.CallToolResult{}, nil
			})
			_, err := handler(ctx, mcp.CallToolRequest{})

			return called, err
		},
		"resource middleware": func(ctx context.Context) (bool, error) {
			called := false
			handler := auth.ResourceAuthenticationMiddleware()(func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				called = true
				return nil, nil
			})
			_, err := handler(ctx, mcp.ReadResourceRequest{})

			return called, err
		},
		"prompt handler": func(ctx context.Context) (bool, error) {
			called := false
			handler := auth.AuthenticatedPromptHandlerFunc(func(_ context.Context, _ mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				called = true
				return &mcp.GetPromptResult{}, nil
			})
			_, err := handler(ctx, mcp.GetPromptRequest{})

			return called, err
		},
	}
}

func TestAuthenticationChecks(t *testing.T) {
	authError := errors.New("TheHive authentication failed: invalid credentials")

	// buildCtx constructs the per-scenario request context from the scenario's
	// declared markers, keeping context out of the test-case table (godre:S8242).
	scenarios := []struct {
		name        string
		validated   bool
		authErr     error
		expectCall  bool
		expectedErr error
	}{
		{
			name:       "allows request when authentication was validated",
			validated:  true,
			expectCall: true,
		},
		{
			name:        "blocks request when auth error exists",
			authErr:     authError,
			expectedErr: authError,
		},
		{
			// No auth error and no validation marker: the request must still
			// be denied, never silently allowed (RandoriSec 5.5)
			name:        "blocks request when validation marker is absent (fail closed)",
			expectedErr: auth.ErrAuthenticationNotValidated,
		},
		{
			name:        "auth error takes precedence over missing marker",
			authErr:     authError,
			expectedErr: authError,
		},
	}

	buildCtx := func(validated bool, authErr error) context.Context {
		ctx := context.Background()
		if validated {
			ctx = context.WithValue(ctx, types.AuthValidatedCtxKey, true)
		}

		if authErr != nil {
			ctx = context.WithValue(ctx, types.AuthErrorCtxKey, authErr)
		}

		return ctx
	}

	for wrapperName, invoke := range authWrappers() {
		for _, scenario := range scenarios {
			t.Run(wrapperName+" "+scenario.name, func(t *testing.T) {
				called, err := invoke(buildCtx(scenario.validated, scenario.authErr))

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

package bootstrap

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetHTTPAuthContextFunc_TheHiveURL(t *testing.T) {
	tests := []struct {
		name        string
		headerValue string
		optionsURL  string
		expectedURL string
		expectInCtx bool
	}{
		{
			name:        "URL from header takes precedence",
			headerValue: "https://header.thehive.com",
			optionsURL:  "https://default.thehive.com",
			expectedURL: "https://header.thehive.com",
			expectInCtx: true,
		},
		{
			name:        "Falls back to options URL when header empty",
			headerValue: "",
			optionsURL:  "https://default.thehive.com",
			expectedURL: "https://default.thehive.com",
			expectInCtx: true,
		},
		{
			name:        "No URL in context when both empty",
			headerValue: "",
			optionsURL:  "",
			expectedURL: "",
			expectInCtx: false,
		},
		{
			name:        "Header value used when options empty",
			headerValue: "https://only-header.thehive.com",
			optionsURL:  "",
			expectedURL: "https://only-header.thehive.com",
			expectInCtx: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test options
			options := &types.TheHiveMcpDefaultOptions{
				TheHiveURL:          tt.optionsURL,
				TheHiveAPIKey:       "test-api-key",
				TheHiveOrganisation: "test-org",
			}

			// Create test request
			req := httptest.NewRequest("POST", "/mcp", nil)
			if tt.headerValue != "" {
				req.Header.Set(string(types.HeaderKeyTheHiveURL), tt.headerValue)
			}

			// Get the auth context function and call it
			authFunc := GetHTTPAuthContextFunc(options)
			ctx := authFunc(context.Background(), req)

			// Check if URL is in context
			urlFromCtx, hasURL := ctx.Value(types.HiveURLCtxKey).(string)

			if tt.expectInCtx {
				require.True(t, hasURL, "Expected URL to be in context")
				assert.Equal(t, tt.expectedURL, urlFromCtx, "URL in context should match expected")
			} else {
				// When no URL is expected, either the key is not present or the value is empty
				if hasURL {
					assert.Empty(t, urlFromCtx, "URL should be empty when not expected")
				}
			}
		})
	}
}

func TestGetHTTPAuthContextFunc_AllHeaders(t *testing.T) {
	options := &types.TheHiveMcpDefaultOptions{
		TheHiveURL:          "https://default.thehive.com",
		TheHiveAPIKey:       "default-key",
		TheHiveOrganisation: "default-org",
	}

	req := httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set(string(types.HeaderKeyTheHiveURL), "https://header.thehive.com")
	req.Header.Set(string(types.HeaderKeyTheHiveAPIKey), "header-api-key")
	req.Header.Set(string(types.HeaderKeyTheHiveOrganisation), "header-org")

	authFunc := GetHTTPAuthContextFunc(options)
	ctx := authFunc(context.Background(), req)

	// Verify all values are set correctly from headers
	assert.Equal(t, "https://header.thehive.com", ctx.Value(types.HiveURLCtxKey))
	assert.Equal(t, "header-api-key", ctx.Value(types.HiveAPIKeyCtxKey))
	assert.Equal(t, "header-org", ctx.Value(types.HiveOrgCtxKey))
}

func TestGetHTTPAuthContextFunc_AuthorizationHeader(t *testing.T) {
	options := &types.TheHiveMcpDefaultOptions{
		TheHiveURL: "https://test.thehive.com",
	}

	tests := []struct {
		name        string
		authHeader  string
		expectedKey string
	}{
		{
			name:        "Bearer token",
			authHeader:  "Bearer test-token-123",
			expectedKey: "test-token-123",
		},
		{
			name:        "bearer lowercase",
			authHeader:  "bearer test-token-456",
			expectedKey: "test-token-456",
		},
		{
			name:        "Direct token",
			authHeader:  "direct-token-789",
			expectedKey: "direct-token-789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/mcp", nil)
			req.Header.Set("Authorization", tt.authHeader)
			req.Header.Set(string(types.HeaderKeyTheHiveURL), "https://test.thehive.com")

			authFunc := GetHTTPAuthContextFunc(options)
			ctx := authFunc(context.Background(), req)

			apiKey := ctx.Value(types.HiveAPIKeyCtxKey).(string)
			assert.Equal(t, tt.expectedKey, apiKey)
		})
	}
}

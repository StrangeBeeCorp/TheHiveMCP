package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/auth"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// fakeTheHive is an httptest stand-in for a TheHive instance that answers the
// credential validation endpoint and records every request it receives.
type fakeTheHive struct {
	server       *httptest.Server
	requests     atomic.Int32
	mu           sync.Mutex
	lastAuth     string
	failNextAuth atomic.Bool
}

func newFakeTheHive(t *testing.T) *fakeTheHive {
	t.Helper()

	fake := &fakeTheHive{}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.requests.Add(1)
		fake.mu.Lock()
		fake.lastAuth = r.Header.Get("Authorization")
		fake.mu.Unlock()

		if fake.failNextAuth.Load() {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if r.URL.Path == "/api/v1/user/current" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"_id":"~1","_createdBy":"admin","_createdAt":1,"login":"test","name":"Test User","hasKey":true,"hasPassword":false,"hasMFA":false,"locked":false,"profile":"analyst","organisation":"test-org","type":"Normal","extraData":{}}`)

			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(fake.server.Close)

	return fake
}

func (f *fakeTheHive) lastAuthorization() string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.lastAuth
}

func authContextForRequest(options *types.TheHiveMcpDefaultOptions, headers map[string]string) context.Context {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", nil)
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	return GetHTTPAuthContextFunc(options)(context.Background(), req)
}

func assertAuthRejected(ctx context.Context, t *testing.T, msgAndArgs ...any) {
	t.Helper()

	authErr, _ := ctx.Value(types.AuthErrorCtxKey).(error)
	require.Error(t, authErr, msgAndArgs...)

	validated, _ := ctx.Value(types.AuthValidatedCtxKey).(bool)
	assert.False(t, validated, msgAndArgs...)
}

func assertAuthValidated(ctx context.Context, t *testing.T, msgAndArgs ...any) {
	t.Helper()

	authErr, _ := ctx.Value(types.AuthErrorCtxKey).(error)
	require.NoError(t, authErr, msgAndArgs...)

	validated, _ := ctx.Value(types.AuthValidatedCtxKey).(bool)
	assert.True(t, validated, msgAndArgs...)
}

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
			optionsURL:  testDefaultHiveURL,
			expectedURL: "https://header.thehive.com",
			expectInCtx: true,
		},
		{
			name:        "Falls back to options URL when header empty",
			headerValue: "",
			optionsURL:  testDefaultHiveURL,
			expectedURL: testDefaultHiveURL,
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
			options := &types.TheHiveMcpDefaultOptions{
				TheHiveURL:          tt.optionsURL,
				TheHiveAPIKey:       "test-api-key",
				TheHiveOrganisation: "test-org",
			}

			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", nil)
			if tt.headerValue != "" {
				req.Header.Set(string(types.HeaderKeyTheHiveURL), tt.headerValue)
			}

			authFunc := GetHTTPAuthContextFunc(options)
			ctx := authFunc(context.Background(), req)

			urlFromCtx, hasURL := ctx.Value(types.HiveURLCtxKey).(string)

			if tt.expectInCtx {
				require.True(t, hasURL, "Expected URL to be in context")
				assert.Equal(t, tt.expectedURL, urlFromCtx, "URL in context should match expected")
			} else if hasURL {
				// Absent key or empty value both satisfy "no URL".
				assert.Empty(t, urlFromCtx, "URL should be empty when not expected")
			}
		})
	}
}

func TestGetHTTPAuthContextFunc_AllHeaders(t *testing.T) {
	options := &types.TheHiveMcpDefaultOptions{
		TheHiveURL:          testDefaultHiveURL,
		TheHiveAPIKey:       "default-key",
		TheHiveOrganisation: "default-org",
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", nil)
	req.Header.Set(string(types.HeaderKeyTheHiveURL), "https://header.thehive.com")
	req.Header.Set(string(types.HeaderKeyTheHiveAPIKey), "header-api-key")
	req.Header.Set(string(types.HeaderKeyTheHiveOrganisation), "header-org")

	authFunc := GetHTTPAuthContextFunc(options)
	ctx := authFunc(context.Background(), req)

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
			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", nil)
			req.Header.Set("Authorization", tt.authHeader)
			req.Header.Set(string(types.HeaderKeyTheHiveURL), "https://test.thehive.com")

			authFunc := GetHTTPAuthContextFunc(options)
			ctx := authFunc(context.Background(), req)

			apiKey, ok := ctx.Value(types.HiveAPIKeyCtxKey).(string)
			require.True(t, ok)
			assert.Equal(t, tt.expectedKey, apiKey)
		})
	}
}

func clientHeaders(hiveURL string) map[string]string {
	headers := map[string]string{
		string(types.HeaderKeyTheHiveAPIKey): "client-key",
	}
	if hiveURL != "" {
		headers[string(types.HeaderKeyTheHiveURL)] = hiveURL
	}

	return headers
}

func TestGetHTTPAuthContextFunc_URLAllowlist(t *testing.T) {
	t.Run("header URL matching allowlist is allowed", func(t *testing.T) {
		hive := newFakeTheHive(t)
		options := &types.TheHiveMcpDefaultOptions{
			TheHiveURLAllowlist: []string{hive.server.URL},
		}

		ctx := authContextForRequest(options, clientHeaders(hive.server.URL))

		assertAuthValidated(ctx, t)
		assert.Equal(t, int32(1), hive.requests.Load())
	})

	t.Run("header URL not in allowlist is rejected before any outbound call", func(t *testing.T) {
		attacker := newFakeTheHive(t)
		options := &types.TheHiveMcpDefaultOptions{
			TheHiveURL: "https://legit.thehive.example.com",
		}

		ctx := authContextForRequest(options, clientHeaders(attacker.server.URL))

		assertAuthRejected(ctx, t)
		assert.Equal(t, int32(0), attacker.requests.Load(), "attacker-controlled URL must never receive a request")
	})

	t.Run("empty allowlist permits only the server's own URL", func(t *testing.T) {
		hive := newFakeTheHive(t)
		attacker := newFakeTheHive(t)
		options := &types.TheHiveMcpDefaultOptions{
			TheHiveURL: hive.server.URL,
		}

		rejectedCtx := authContextForRequest(options, clientHeaders(attacker.server.URL))
		assertAuthRejected(rejectedCtx, t)
		assert.Equal(t, int32(0), attacker.requests.Load())

		allowedCtx := authContextForRequest(options, clientHeaders(hive.server.URL))
		assertAuthValidated(allowedCtx, t)
		assert.Equal(t, int32(1), hive.requests.Load())
	})

	t.Run("host suffix does not bypass the allowlist", func(t *testing.T) {
		options := &types.TheHiveMcpDefaultOptions{
			TheHiveURLAllowlist: []string{testHiveComURL},
		}

		for _, evil := range []string{
			"https://thehive.com.evil.test",
			"https://evil-thehive.com",
			"https://thehive.com@evil.test",
		} {
			ctx := authContextForRequest(options, clientHeaders(evil))
			assertAuthRejected(ctx, t, "URL %q must be rejected", evil)
		}
	})
}

func TestGetHTTPAuthContextFunc_EnvCredentialFallback(t *testing.T) {
	t.Run("fails closed without credentials when fallback is off (default)", func(t *testing.T) {
		hive := newFakeTheHive(t)
		options := &types.TheHiveMcpDefaultOptions{
			TheHiveURL:    hive.server.URL,
			TheHiveAPIKey: testEnvSecretKey,
		}

		ctx := authContextForRequest(options, nil)

		assertAuthRejected(ctx, t)
		assert.Equal(t, int32(0), hive.requests.Load(), "env API key must never be sent on behalf of an unauthenticated request")

		apiKey, _ := ctx.Value(types.HiveAPIKeyCtxKey).(string)
		assert.Empty(t, apiKey, "env API key must not leak into the request context")
	})

	t.Run("uses env credentials when fallback is explicitly enabled", func(t *testing.T) {
		hive := newFakeTheHive(t)
		options := &types.TheHiveMcpDefaultOptions{
			TheHiveURL:                 hive.server.URL,
			TheHiveAPIKey:              testEnvSecretKey,
			AllowEnvCredentialFallback: true,
		}

		ctx := authContextForRequest(options, nil)

		assertAuthValidated(ctx, t)
		assert.Equal(t, int32(1), hive.requests.Load())
		assert.Equal(t, "Bearer env-secret-key", hive.lastAuthorization())
	})

	t.Run("client-supplied credentials are used regardless of fallback setting", func(t *testing.T) {
		hive := newFakeTheHive(t)
		options := &types.TheHiveMcpDefaultOptions{
			TheHiveURL:    hive.server.URL,
			TheHiveAPIKey: testEnvSecretKey,
		}

		ctx := authContextForRequest(options, clientHeaders(""))

		assertAuthValidated(ctx, t)
		assert.Equal(t, "Bearer client-key", hive.lastAuthorization())
	})
}

// Regression test for RandoriSec 5.5: an empty X-TheHive-Url with no
// configured server URL must be denied, not silently allowed.
func TestGetHTTPAuthContextFunc_EmptyURLFailsClosed(t *testing.T) {
	ctx := authContextForRequest(&types.TheHiveMcpDefaultOptions{}, map[string]string{
		string(types.HeaderKeyTheHiveURL):    "",
		string(types.HeaderKeyTheHiveAPIKey): "client-key",
	})

	assertAuthRejected(ctx, t)

	handler := auth.AuthenticationMiddleware()(func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		t.Fatal("tool handler must not be called for an unauthenticated request")
		return nil, errUnexpectedHandlerCall
	})
	_, err := handler(ctx, mcp.CallToolRequest{})
	assert.Error(t, err)
}

func TestGetHTTPAuthContextFunc_ValidationCache(t *testing.T) {
	invoke := func(authFunc func(context.Context, *http.Request) context.Context, apiKey string) context.Context {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", nil)
		req.Header.Set(string(types.HeaderKeyTheHiveAPIKey), apiKey)

		return authFunc(context.Background(), req)
	}

	t.Run("identical credentials within TTL validate upstream once", func(t *testing.T) {
		hive := newFakeTheHive(t)
		authFunc := GetHTTPAuthContextFunc(&types.TheHiveMcpDefaultOptions{
			TheHiveURL:             hive.server.URL,
			AuthValidationCacheTTL: "150ms",
		})

		for i := range 2 {
			assertAuthValidated(invoke(authFunc, "client-key"), t, "request %d", i)
		}

		assert.Equal(t, int32(1), hive.requests.Load(), "second request within TTL must hit the cache")

		time.Sleep(200 * time.Millisecond)
		assertAuthValidated(invoke(authFunc, "client-key"), t)
		assert.Equal(t, int32(2), hive.requests.Load(), "expired cache entry must be re-validated")
	})

	t.Run("different credentials are validated separately", func(t *testing.T) {
		hive := newFakeTheHive(t)
		authFunc := GetHTTPAuthContextFunc(&types.TheHiveMcpDefaultOptions{TheHiveURL: hive.server.URL})

		for _, key := range []string{"key-one", "key-two"} {
			assertAuthValidated(invoke(authFunc, key), t)
		}

		assert.Equal(t, int32(2), hive.requests.Load(), "each credential set must be validated upstream")
	})

	t.Run("failed validations are not cached as successes", func(t *testing.T) {
		hive := newFakeTheHive(t)
		authFunc := GetHTTPAuthContextFunc(&types.TheHiveMcpDefaultOptions{TheHiveURL: hive.server.URL})

		hive.failNextAuth.Store(true)
		assertAuthRejected(invoke(authFunc, "client-key"), t)
		assert.Equal(t, int32(1), hive.requests.Load())

		hive.failNextAuth.Store(false)
		assertAuthValidated(invoke(authFunc, "client-key"), t)
		assert.Equal(t, int32(2), hive.requests.Load())
	})

	t.Run("concurrent requests are safe", func(t *testing.T) {
		hive := newFakeTheHive(t)
		authFunc := GetHTTPAuthContextFunc(&types.TheHiveMcpDefaultOptions{TheHiveURL: hive.server.URL})

		type authOutcome struct {
			err       error
			validated bool
		}

		var wg sync.WaitGroup

		outcomes := make([]authOutcome, 16)

		for i := range 16 {
			wg.Add(1)

			go func(i int) {
				defer wg.Done()

				// Assertions must run on the test goroutine, so only exercise the
				// auth func concurrently here and record the outcome for the main
				// goroutine to validate afterwards.
				resultCtx := invoke(authFunc, fmt.Sprintf("key-%d", i%4))

				authErr, _ := resultCtx.Value(types.AuthErrorCtxKey).(error)
				validated, _ := resultCtx.Value(types.AuthValidatedCtxKey).(bool)
				outcomes[i] = authOutcome{err: authErr, validated: validated}
			}(i)
		}

		wg.Wait()

		for i, outcome := range outcomes {
			require.NoError(t, outcome.err, "request %d", i)
			assert.True(t, outcome.validated, "request %d", i)
		}
	})
}

func TestStartHTTPServer_InvalidAllowlist(t *testing.T) {
	options := &types.TheHiveMcpDefaultOptions{
		BindAddr:            "127.0.0.1:0",
		TheHiveURLAllowlist: []string{"ftp://not-http.example.com"},
	}

	err := StartHTTPServer(GetMCPServer(), options)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "allowlist")
}

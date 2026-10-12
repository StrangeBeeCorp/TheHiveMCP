package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

func TestGetHTTPAuthContextFunc_EnvCredentialFallback(t *testing.T) {
	t.Parallel()

	t.Run("fails closed without credentials when fallback is off (default)", func(t *testing.T) {
		t.Parallel()

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
		t.Parallel()

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

	// Regression: with fallback on, the env key was the default of each key
	// header, so a key sent only as a Bearer token was overwritten by it.
	t.Run("client-supplied credentials are used regardless of fallback setting", func(t *testing.T) {
		t.Parallel()

		for _, fallback := range []bool{false, true} {
			for header, value := range map[string]string{
				string(types.HeaderKeyTheHiveAPIKey): testClientKey,
				headerAuthorization:                  "Bearer " + testClientKey,
			} {
				t.Run(fmt.Sprintf("%s with fallback %t", header, fallback), func(t *testing.T) {
					t.Parallel()

					hive := newFakeTheHive(t)
					options := &types.TheHiveMcpDefaultOptions{
						TheHiveURL:                 hive.server.URL,
						TheHiveAPIKey:              testEnvSecretKey,
						AllowEnvCredentialFallback: fallback,
					}

					ctx := authContextForRequest(options, map[string]string{header: value})

					assertAuthValidated(ctx, t)
					assert.Equal(t, "Bearer client-key", hive.lastAuthorization())
				})
			}
		}
	})

	// The Basic credentials an authenticating proxy passes on are not a
	// TheHive key: they must never reach TheHive, and must not block the
	// fallback a single-user deployment behind that proxy relies on.
	t.Run("a proxy's Basic credentials are ignored", func(t *testing.T) {
		t.Parallel()

		basic := map[string]string{headerAuthorization: "Basic cHJveHktdXNlcjpwcm94eS1wYXNzd29yZA=="}

		hive := newFakeTheHive(t)
		ctx := authContextForRequest(&types.TheHiveMcpDefaultOptions{
			TheHiveURL: hive.server.URL, TheHiveAPIKey: testEnvSecretKey, AllowEnvCredentialFallback: true,
		}, basic)
		assertAuthValidated(ctx, t)
		assert.Equal(t, "Bearer env-secret-key", hive.lastAuthorization())

		hive = newFakeTheHive(t)
		ctx = authContextForRequest(&types.TheHiveMcpDefaultOptions{TheHiveURL: hive.server.URL, TheHiveAPIKey: testEnvSecretKey}, basic)
		assertAuthRejected(ctx, t)
		assert.Zero(t, hive.requests.Load(), "without fallback the request has no key and must not reach TheHive")
	})

	t.Run("X-TheHive-Api-Key wins over Authorization", func(t *testing.T) {
		t.Parallel()

		hive := newFakeTheHive(t)
		ctx := authContextForRequest(&types.TheHiveMcpDefaultOptions{TheHiveURL: hive.server.URL}, map[string]string{
			headerAuthorization:                  "Bearer other-key",
			string(types.HeaderKeyTheHiveAPIKey): testClientKey,
		})

		assertAuthValidated(ctx, t)
		assert.Equal(t, "Bearer client-key", hive.lastAuthorization())
	})

	t.Run("a client key wins over the env username and password", func(t *testing.T) {
		t.Parallel()

		hive := newFakeTheHive(t)
		ctx := authContextForRequest(&types.TheHiveMcpDefaultOptions{
			TheHiveURL: hive.server.URL, TheHiveUsername: "service", TheHivePassword: "service-password", AllowEnvCredentialFallback: true,
		}, map[string]string{headerAuthorization: "Bearer " + testClientKey})

		assertAuthValidated(ctx, t)
		assert.Equal(t, "Bearer client-key", hive.lastAuthorization())
	})
}

func TestGetHTTPAuthContextFunc_ValidationCache(t *testing.T) {
	t.Parallel()

	invoke := func(authFunc func(context.Context, *http.Request) context.Context, apiKey string) context.Context {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", nil)
		req.Header.Set(string(types.HeaderKeyTheHiveAPIKey), apiKey)

		return authFunc(context.Background(), req)
	}

	t.Run("identical credentials within TTL validate upstream once", func(t *testing.T) {
		t.Parallel()

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
		t.Parallel()

		hive := newFakeTheHive(t)
		authFunc := GetHTTPAuthContextFunc(&types.TheHiveMcpDefaultOptions{TheHiveURL: hive.server.URL})

		for _, key := range []string{"key-one", "key-two"} {
			assertAuthValidated(invoke(authFunc, key), t)
		}

		assert.Equal(t, int32(2), hive.requests.Load(), "each credential set must be validated upstream")
	})

	t.Run("failed validations are not cached as successes", func(t *testing.T) {
		t.Parallel()

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
		t.Parallel()

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

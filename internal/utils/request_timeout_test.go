package utils

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testRequestTimeout = 50 * time.Millisecond

func newTimeoutClient() *http.Client {
	return &http.Client{Transport: &TimeoutTransport{Timeout: testRequestTimeout}}
}

func get(ctx context.Context, t *testing.T, client *http.Client, url string) (*http.Response, error) {
	t.Helper()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)

	return client.Do(req) //nolint:wrapcheck // test helper
}

func TestTimeoutTransport_NoAnswer(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	resp, err := get(t.Context(), t, newTimeoutClient(), server.URL+"/api/v1/query")
	if resp != nil {
		_ = resp.Body.Close()
	}

	timeoutErr, ok := AsTheHiveTimeout(err)
	require.True(t, ok, "expected a TheHive timeout, got %v", err)
	assert.Equal(t, http.MethodGet, timeoutErr.Method)
	assert.Equal(t, "/api/v1/query", timeoutErr.Path)
	assert.Equal(t, testRequestTimeout, timeoutErr.Timeout)
}

func TestTimeoutTransport_StalledBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))

		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}

		<-r.Context().Done()
	}))
	defer server.Close()

	resp, err := get(t.Context(), t, newTimeoutClient(), server.URL)
	require.NoError(t, err, "headers arrive in time")

	defer func() { _ = resp.Body.Close() }()

	_, err = io.ReadAll(resp.Body)

	_, ok := AsTheHiveTimeout(err)
	assert.True(t, ok, "a body that stops arriving is a timeout too, got %v", err)
}

func TestTimeoutTransport_FastAnswer(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	resp, err := get(t.Context(), t, newTimeoutClient(), server.URL)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "ok", string(body))
}

func TestTimeoutTransport_CallerCancellationIsNotATimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	client := &http.Client{Transport: &TimeoutTransport{Timeout: time.Minute}}

	go func() {
		time.Sleep(testRequestTimeout)
		cancel()
	}()

	resp, err := get(ctx, t, client, server.URL)
	if resp != nil {
		_ = resp.Body.Close()
	}

	require.Error(t, err)

	_, ok := AsTheHiveTimeout(err)
	assert.False(t, ok, "the caller cancelled; reporting a TheHive timeout would be wrong")
}

func TestTheHiveTimeoutError_HintsWarnAboutAbandonedWrites(t *testing.T) {
	t.Parallel()

	err := &TheHiveTimeoutError{Method: http.MethodPost, Path: "/api/v1/case", Timeout: 2 * time.Minute}

	assert.Equal(t, "TheHive did not answer within 2m0s", err.Summary())
	assert.Contains(t, err.Error(), "POST /api/v1/case")
	assert.Contains(t, err.Hints()[1], "read the entity back before retrying")
	assert.Contains(t, err.Hints()[2], "THEHIVE_REQUEST_TIMEOUT")
}

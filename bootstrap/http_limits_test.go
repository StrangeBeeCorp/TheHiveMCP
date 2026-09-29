package bootstrap

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

const testReadTimeout = 200 * time.Millisecond

func testHTTPServer(t *testing.T, endpointPath string) *http.Server {
	t.Helper()

	srv, _ := newHTTPServer(GetMCPServer(), &types.TheHiveMcpDefaultOptions{MCPServerEndpointPath: endpointPath})

	return srv
}

func postStatus(t *testing.T, url, body string) int {
	t.Helper()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, strings.NewReader(body))
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode
}

// mcp-go's own Start served with a bare http.Server, which bounds nothing.
func TestHTTPServerBoundsSlowRequests(t *testing.T) {
	t.Parallel()

	srv := testHTTPServer(t, "/mcp")

	assert.Equal(t, readHeaderTimeout, srv.ReadHeaderTimeout)
	assert.Equal(t, readTimeout, srv.ReadTimeout)
	assert.Equal(t, idleTimeout, srv.IdleTimeout)
	assert.Zero(t, srv.WriteTimeout, "a write deadline would cut SSE streams and long tool calls")
}

func TestHTTPServerRefusesAnOversizedBody(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(testHTTPServer(t, "/mcp").Handler)
	t.Cleanup(ts.Close)

	initialize := func(padding int) int {
		body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},` +
			`"clientInfo":{"name":"test","version":"1"},"padding":"` + strings.Repeat("x", padding) + `"}}`

		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/mcp", strings.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)

		defer func() { _ = resp.Body.Close() }()

		return resp.StatusCode
	}

	require.Equal(t, http.StatusOK, initialize(1), "control: the same request under the cap is served")
	assert.GreaterOrEqual(t, initialize(maxRequestBodyBytes), http.StatusBadRequest)
}

// The router must agree with mcp-go on the path, which it normalizes.
func TestHTTPServerRoutesTheNormalizedEndpointPath(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(testHTTPServer(t, "mcp/").Handler)
	t.Cleanup(ts.Close)

	assert.NotEqual(t, http.StatusNotFound, postStatus(t, ts.URL+"/mcp", "{}"))
	assert.Equal(t, http.StatusNotFound, postStatus(t, ts.URL+"/other", "{}"))
}

// A client that sends its headers and then stops sending its body is answered
// and let go instead of holding the connection.
func TestHTTPServerLetsGoOfAStalledBody(t *testing.T) {
	t.Parallel()

	ts := httptest.NewUnstartedServer(testHTTPServer(t, "/mcp").Handler)
	ts.Config.ReadTimeout = testReadTimeout
	ts.Start()
	t.Cleanup(ts.Close)

	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", ts.Listener.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	_, err = io.WriteString(conn, "POST /mcp HTTP/1.1\r\nHost: test\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n{\"json")
	require.NoError(t, err)

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err == nil {
		_ = resp.Body.Close()
	}

	var netErr net.Error
	assert.False(t, errors.As(err, &netErr) && netErr.Timeout(), "the server must not wait for the rest of the body")
}

// What readTimeout relies on: net/http lifts the read deadline once the body is
// read, so a handler running longer than readTimeout keeps its context.
func TestReadTimeoutSparesAHandlerThatOutlivesIt(t *testing.T) {
	t.Parallel()

	ctxErr := make(chan error, 1)
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)

		time.Sleep(3 * testReadTimeout)

		ctxErr <- r.Context().Err()

		w.WriteHeader(http.StatusOK)
	}))
	ts.Config.ReadTimeout = testReadTimeout
	ts.Start()
	t.Cleanup(ts.Close)

	require.Equal(t, http.StatusOK, postStatus(t, ts.URL, "{}"))
	assert.NoError(t, <-ctxErr)
}

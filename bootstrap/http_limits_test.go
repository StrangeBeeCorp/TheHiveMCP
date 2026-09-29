package bootstrap

import (
	"bufio"
	"context"
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

	// chunked sends the body with no declared length, which only the size cap
	// on the read itself can stop.
	initialize := func(padding int, chunked bool) (int, string) {
		body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},` +
			`"clientInfo":{"name":"test","version":"1"},"padding":"` + strings.Repeat("x", padding) + `"}}`

		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/mcp", strings.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")

		if chunked {
			req.ContentLength = -1
		}

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)

		defer func() { _ = resp.Body.Close() }()

		answer, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		return resp.StatusCode, string(answer)
	}

	status, _ := initialize(1, false)
	require.Equal(t, http.StatusOK, status, "control: the same request under the cap is served")

	status, _ = initialize(maxRequestBodyBytes, false)
	assert.Equal(t, http.StatusRequestEntityTooLarge, status, "a declared oversize is refused before any of it is read")

	status, answer := initialize(maxRequestBodyBytes, true)
	assert.Equal(t, http.StatusBadRequest, status, "an undeclared oversize is cut off at the cap")
	assert.Contains(t, answer, "request body too large")
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
// and let go instead of holding the connection. The server's own settings are
// kept, with ReadTimeout shortened so the test does not wait 30 s.
func TestHTTPServerLetsGoOfAStalledBody(t *testing.T) {
	t.Parallel()

	srv := testHTTPServer(t, "/mcp")
	ts := httptest.NewUnstartedServer(srv.Handler)
	ts.Config.ReadHeaderTimeout = srv.ReadHeaderTimeout
	ts.Config.ReadTimeout = testReadTimeout
	ts.Start()
	t.Cleanup(ts.Close)

	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", ts.Listener.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	// Host: localhost, or mcp-go's DNS-rebinding guard answers 403 before
	// reading the body at all.
	_, err = io.WriteString(conn, "POST /mcp HTTP/1.1\r\nHost: localhost\r\nContent-Type: application/json\r\n"+
		"Accept: application/json, text/event-stream\r\nContent-Length: 100\r\n\r\n{\"json")
	require.NoError(t, err)

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	require.NoError(t, err, "the server must answer instead of waiting for the rest of the body")

	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "the body read fails, and mcp-go says so")
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
	ts.Config.ReadHeaderTimeout = readHeaderTimeout // else it falls back to the short ReadTimeout
	ts.Config.ReadTimeout = testReadTimeout
	ts.Start()
	t.Cleanup(ts.Close)

	require.Equal(t, http.StatusOK, postStatus(t, ts.URL, "{}"))
	assert.NoError(t, <-ctxErr)
}

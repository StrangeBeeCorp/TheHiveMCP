package utils

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"
)

// recordingTransport is a fake http.RoundTripper that records whether it was
// invoked, so tests can assert a refused request never reaches TheHive.
type recordingTransport struct {
	calls int
}

func (rt *recordingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	rt.calls++

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(nil)),
		Header:     make(http.Header),
	}, nil
}

// clientInfoSession implements ClientSession + SessionWithClientInfo. Whether it
// advertises elicitation is controlled by elicitationCapable.
type clientInfoSession struct {
	elicitationCapable bool
}

func (s *clientInfoSession) SessionID() string                                   { return "test-session" }
func (s *clientInfoSession) Initialize()                                         {}
func (s *clientInfoSession) Initialized() bool                                   { return true }
func (s *clientInfoSession) NotificationChannel() chan<- mcp.JSONRPCNotification { return nil }
func (s *clientInfoSession) GetClientInfo() mcp.Implementation                   { return mcp.Implementation{} }
func (s *clientInfoSession) SetClientInfo(mcp.Implementation)                    {}
func (s *clientInfoSession) SetClientCapabilities(mcp.ClientCapabilities)        {}
func (s *clientInfoSession) GetClientCapabilities() mcp.ClientCapabilities {
	caps := mcp.ClientCapabilities{}
	if s.elicitationCapable {
		caps.Elicitation = &struct{}{}
	}

	return caps
}

// elicitingSession also implements SessionWithElicitation, returning a canned
// response, so the "user accepts/declines" paths can be exercised.
type elicitingSession struct {
	clientInfoSession

	result *mcp.ElicitationResult
	err    error
}

func (s *elicitingSession) RequestElicitation(context.Context, mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
	return s.result, s.err
}

// ctxWithSession attaches a client session to the context the way the MCP
// server does. The server receiver is never dereferenced by WithContext or
// RequestElicitation, so a freshly constructed server is sufficient.
func ctxWithSession(session server.ClientSession) context.Context {
	srv := server.NewMCPServer("test", "1.0.0", server.WithElicitation())
	return srv.WithContext(context.Background(), session)
}

func newRequest(ctx context.Context, t *testing.T, method, path string) *http.Request {
	t.Helper()

	req, err := http.NewRequestWithContext(ctx, method, "http://thehive.example.com"+path, nil)
	require.NoError(t, err)

	return req
}

// closeBody closes a response body when present, ignoring the close error.
func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

func TestRequiresElicitation(t *testing.T) {
	e := &ElicitationTransport{}

	tests := []struct {
		method   string
		endpoint string
		want     bool
	}{
		{methodPost, "/api/v1/case", true},
		{methodPatch, pathCaseByID, true},
		{methodDelete, pathCaseByID, true},
		{"post", "/api/v1/alert", true},
		{"GET", pathCaseByID, false},
		{"HEAD", pathCaseByID, false},
		{"PUT", pathCaseByID, false},
		{methodPost, pathQuery, false},
		{methodPatch, pathQuery, false},
		{methodDelete, pathQuery, false},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, e.requiresElicitation(tt.method, tt.endpoint), "%s %s", tt.method, tt.endpoint)
	}
}

// TestRoundTrip_NoCapabilityProceeds documents that a client advertising no
// elicitation capability is allowed to proceed: elicitation is not the
// authorization layer (permissions and the API key are), so its absence does
// not block modifying requests.
func TestRoundTrip_NoCapabilityProceeds(t *testing.T) {
	rt := &recordingTransport{}
	e := &ElicitationTransport{Transport: rt}

	resp, err := e.RoundTrip(newRequest(context.Background(), t, methodDelete, pathCaseByID))
	defer closeBody(resp)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 1, rt.calls)
}

func TestRoundTrip_NonModifyingAlwaysProceeds(t *testing.T) {
	rt := &recordingTransport{}
	e := &ElicitationTransport{Transport: rt}

	resp, err := e.RoundTrip(newRequest(context.Background(), t, "GET", pathCaseByID))
	defer closeBody(resp)

	require.NoError(t, err)
	require.Equal(t, 1, rt.calls)
}

func TestRoundTrip_QueryEndpointProceeds(t *testing.T) {
	rt := &recordingTransport{}
	e := &ElicitationTransport{Transport: rt}

	// POST to /api/v1/query is a read disguised as POST; must not be gated
	resp, err := e.RoundTrip(newRequest(context.Background(), t, methodPost, pathQuery))
	defer closeBody(resp)

	require.NoError(t, err)
	require.Equal(t, 1, rt.calls)
}

// TestRoundTrip_AdvertisedButUnsupportedMidflightDenies is the behavior this
// change adds: the session advertises the elicitation capability (so we enter
// handleElicitation) but does not implement SessionWithElicitation, so
// RequestElicitation returns ErrElicitationNotSupported. The request must be
// refused, not run unconfirmed.
func TestRoundTrip_AdvertisedButUnsupportedMidflightDenies(t *testing.T) {
	rt := &recordingTransport{}
	e := &ElicitationTransport{Transport: rt}

	ctx := ctxWithSession(&clientInfoSession{elicitationCapable: true})

	resp, err := e.RoundTrip(newRequest(ctx, t, methodDelete, pathCaseByID))
	defer closeBody(resp)

	require.Error(t, err)
	require.Nil(t, resp)
	require.Equal(t, 0, rt.calls, "underlying transport must not be called when confirmation fails")
	require.Contains(t, err.Error(), "operation refused")
	require.Contains(t, err.Error(), "DELETE")
}

// --- Confirmation IS available: accept / decline (regression guards) ---

func TestRoundTrip_UserAcceptsProceeds(t *testing.T) {
	rt := &recordingTransport{}
	e := &ElicitationTransport{Transport: rt}

	session := &elicitingSession{
		clientInfoSession: clientInfoSession{elicitationCapable: true},
		result: &mcp.ElicitationResult{
			ElicitationResponse: mcp.ElicitationResponse{Action: mcp.ElicitationResponseActionAccept},
		},
	}

	resp, err := e.RoundTrip(newRequest(ctxWithSession(session), t, methodDelete, pathCaseByID))
	defer closeBody(resp)

	require.NoError(t, err)
	require.Equal(t, 1, rt.calls)
}

func TestRoundTrip_UserDeclinesIsRefused(t *testing.T) {
	rt := &recordingTransport{}
	e := &ElicitationTransport{Transport: rt}

	session := &elicitingSession{
		clientInfoSession: clientInfoSession{elicitationCapable: true},
		result: &mcp.ElicitationResult{
			ElicitationResponse: mcp.ElicitationResponse{Action: mcp.ElicitationResponseActionDecline},
		},
	}

	resp, err := e.RoundTrip(newRequest(ctxWithSession(session), t, methodDelete, pathCaseByID))
	defer closeBody(resp)

	require.Error(t, err)
	require.Equal(t, 0, rt.calls)
	require.Contains(t, err.Error(), "declined by user")
}

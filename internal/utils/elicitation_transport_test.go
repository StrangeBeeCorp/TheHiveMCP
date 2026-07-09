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

// recordingTransport counts invocations so tests can assert a refused request
// never reaches TheHive.
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

// clientInfoSession implements ClientSession + SessionWithClientInfo;
// elicitationCapable toggles whether it advertises the elicitation capability.
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

// elicitingSession adds SessionWithElicitation, returning a canned response, to
// exercise the accept/decline paths.
type elicitingSession struct {
	clientInfoSession

	result *mcp.ElicitationResult
	err    error
}

func (s *elicitingSession) RequestElicitation(context.Context, mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
	return s.result, s.err
}

// ctxWithSession attaches a client session to the context. WithContext /
// RequestElicitation never dereference the server, so a freshly constructed one
// suffices.
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
	t.Parallel()

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

// A client advertising no elicitation capability proceeds: elicitation is not the
// authorization layer (permissions and the API key are), so its absence must not
// block modifying requests.
func TestRoundTrip_NoCapabilityProceeds(t *testing.T) {
	t.Parallel()

	rt := &recordingTransport{}
	e := &ElicitationTransport{Transport: rt}

	resp, err := e.RoundTrip(newRequest(context.Background(), t, methodDelete, pathCaseByID))
	defer closeBody(resp)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 1, rt.calls)
}

func TestRoundTrip_NonModifyingAlwaysProceeds(t *testing.T) {
	t.Parallel()

	rt := &recordingTransport{}
	e := &ElicitationTransport{Transport: rt}

	resp, err := e.RoundTrip(newRequest(context.Background(), t, "GET", pathCaseByID))
	defer closeBody(resp)

	require.NoError(t, err)
	require.Equal(t, 1, rt.calls)
}

func TestRoundTrip_QueryEndpointProceeds(t *testing.T) {
	t.Parallel()

	rt := &recordingTransport{}
	e := &ElicitationTransport{Transport: rt}

	// POST to /api/v1/query is a read disguised as POST; must not be gated
	resp, err := e.RoundTrip(newRequest(context.Background(), t, methodPost, pathQuery))
	defer closeBody(resp)

	require.NoError(t, err)
	require.Equal(t, 1, rt.calls)
}

// Advertises elicitation but doesn't implement SessionWithElicitation, so
// RequestElicitation fails midflight: the request must be refused, not run
// unconfirmed.
func TestRoundTrip_AdvertisedButUnsupportedMidflightDenies(t *testing.T) {
	t.Parallel()

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

func TestRoundTrip_UserAcceptsProceeds(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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

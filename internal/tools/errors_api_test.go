package tools

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// answeringQuery runs a TheHive query against a server that answers 400 with
// body, and returns the response and the client's error.
func answeringQuery(t *testing.T, body string) (*http.Response, error) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	cfg := thehive.NewConfiguration()
	cfg.Servers = thehive.ServerConfigurations{{URL: server.URL}}

	_, resp, err := thehive.NewAPIClient(cfg).QueryAndExportAPI.QueryAPI(t.Context()).InputQuery(thehive.InputQuery{}).Execute()
	require.Error(t, err)

	return resp, err //nolint:wrapcheck // the client's own error is what these tests inspect
}

// Regression (DL-8119): TheHive's 400 carried an "errors" field the client's
// error model lacked, so every `.Cause(err).API(resp)` error read
// `json: unknown field "errors"` as its cause. The client now decodes that body
// (next test); a body it still cannot decode, a known field of the wrong type,
// has a cause that names TheHive's status, and TheHive's message stays in
// apiResponse, once.
func TestToolErrorCauseNamesTheHiveStatusWhenTheClientCannotDecode(t *testing.T) {
	t.Parallel()

	resp, err := answeringQuery(t,
		`{"type":12,"message":"Invalid field nosuchfield","errors":[{"name":"nosuchfield"}]}`)
	require.Contains(t, err.Error(), "json: ", "precondition: the client fails to decode this body")

	toolErr := NewToolError("failed to run the query").Cause(err).API(resp)
	decoded := decodeToolError(t, toolErr.Error())

	assert.Equal(t, "TheHive answered 400 Bad Request: see its message in the API response", decoded["cause"])
	apiResponse, ok := decoded["apiResponse"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Invalid field nosuchfield", apiResponse["message"])
}

// The client fix of DL-8632: TheHive's real 400 body, with its "errors" field,
// decodes, so the cause is the client's own message, and TheHive's message stays
// in apiResponse.
func TestToolErrorCauseIsTheClientMessageWhenItDecodesTheHive400Body(t *testing.T) {
	t.Parallel()

	resp, err := answeringQuery(t,
		`{"type":"BadRequest","message":"Invalid field nosuchfield","errors":[{"name":"nosuchfield"}]}`)
	require.NotContains(t, err.Error(), "json: ", "the client decodes TheHive's 400 body")

	toolErr := NewToolError("failed to run the query").Cause(err).API(resp)
	decoded := decodeToolError(t, toolErr.Error())

	assert.Equal(t, err.Error(), decoded["cause"])
	apiResponse, ok := decoded["apiResponse"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Invalid field nosuchfield", apiResponse["message"])
}

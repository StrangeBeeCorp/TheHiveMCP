package tools

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression (DL-8119): TheHive's 400 carried an "errors" field the client's
// error model lacks, so every `.Cause(err).API(resp)` error read
// `json: unknown field "errors"` as its cause. The cause now names TheHive's
// status, and TheHive's message stays in apiResponse, once.
func TestToolErrorCauseNamesTheHiveStatusWhenTheClientCannotDecode(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"type":"BadRequest","message":"Invalid field nosuchfield","errors":[{"name":"nosuchfield"}]}`))
	}))
	t.Cleanup(server.Close)

	cfg := thehive.NewConfiguration()
	cfg.Servers = thehive.ServerConfigurations{{URL: server.URL}}

	_, resp, err := thehive.NewAPIClient(cfg).QueryAndExportAPI.QueryAPI(t.Context()).InputQuery(thehive.InputQuery{}).Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown field", "precondition: the client fails to decode this body")

	toolErr := NewToolError("failed to run the query").Cause(err).API(resp)
	decoded := decodeToolError(t, toolErr.Error())

	assert.Equal(t, "TheHive answered 400 Bad Request: see its message in the API response", decoded["cause"])
	apiResponse, ok := decoded["apiResponse"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Invalid field nosuchfield", apiResponse["message"])
}

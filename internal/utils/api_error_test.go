package utils

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// queryAnswering runs a TheHive query against a server that answers status and
// body, and returns the client's error.
func queryAnswering(t *testing.T, status int, body string) error {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	cfg := thehive.NewConfiguration()
	cfg.Servers = thehive.ServerConfigurations{{URL: server.URL}}

	_, resp, err := thehive.NewAPIClient(cfg).QueryAndExportAPI.QueryAPI(t.Context()).InputQuery(thehive.InputQuery{}).Execute()
	if resp != nil {
		_ = resp.Body.Close()
	}

	require.Error(t, err)

	return err //nolint:wrapcheck // the client's own error is what these tests inspect
}

// Regression: an invalid filter field made TheHive answer 400 with an "errors"
// field the client's error model lacks, and the cause read
// `json: unknown field "errors"` instead of TheHive's message.
func TestReadableAPIErrorShowsTheHiveAnswer(t *testing.T) {
	t.Parallel()

	err := queryAnswering(t, http.StatusBadRequest,
		`{"type":"BadRequest","message":"Invalid field nosuchfield","errors":[{"name":"nosuchfield"}]}`)
	require.Contains(t, err.Error(), "json: ", "precondition: the client fails to decode this body")

	readable := ReadableAPIError(err)
	assert.Contains(t, readable.Error(), "Invalid field nosuchfield")
	assert.NotContains(t, readable.Error(), "unknown field")

	var apiErr *thehive.GenericOpenAPIError
	assert.ErrorAs(t, readable, &apiErr, "the client's error stays reachable")
}

func TestReadableAPIErrorKeepsOtherErrors(t *testing.T) {
	t.Parallel()

	decoded := queryAnswering(t, http.StatusBadRequest, `{"type":"BadRequest","message":"Invalid filter"}`)
	assert.Same(t, decoded, ReadableAPIError(decoded), "a body the client decoded keeps its error")

	plain := fmt.Errorf("request to TheHive: %w", context.DeadlineExceeded)
	assert.Same(t, plain, ReadableAPIError(plain))

	decodeFailure := queryAnswering(t, http.StatusBadRequest, `{"type":"BadRequest","message":"x","errors":[]}`)
	wrapped := fmt.Errorf("failed to search cases: %w", decodeFailure)
	assert.Same(t, wrapped, ReadableAPIError(wrapped), "a caller's message around the client's error is kept")
}

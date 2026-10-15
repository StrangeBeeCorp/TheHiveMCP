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

// The parts of a response StatusCause reads.
var (
	badRequest = &http.Response{StatusCode: http.StatusBadRequest, Status: "400 Bad Request"}
	ok200      = &http.Response{StatusCode: http.StatusOK, Status: "200 OK"}
)

// Regression: an invalid filter field made TheHive answer 400 with an "errors"
// field the client's error model lacked, and the cause read
// `json: unknown field "errors"` instead of pointing at TheHive's answer. The
// client now knows that body (see the next test); a body it still cannot decode,
// a known field of the wrong type, keeps the cause pointing at TheHive's answer.
func TestStatusCauseNamesTheHiveStatus(t *testing.T) {
	t.Parallel()

	err := queryAnswering(t, http.StatusBadRequest,
		`{"type":12,"message":"Invalid field nosuchfield","errors":[{"name":"nosuchfield"}]}`)
	require.Contains(t, err.Error(), "json: ", "precondition: the client fails to decode this body")

	cause := StatusCause(err, badRequest)
	assert.Equal(t, "TheHive answered 400 Bad Request: see its message in the API response", cause.Error())
	assert.NotContains(t, cause.Error(), "nosuchfield", "the body is not copied into the cause")

	var apiErr *thehive.GenericOpenAPIError
	assert.ErrorAs(t, cause, &apiErr, "the client's error stays reachable")
}

// The client fix of DL-8632: TheHive's real 400 body, with its "errors" field,
// decodes, so the cause is the client's own message and StatusCause leaves it.
func TestClientDecodesTheHive400BodyWithErrors(t *testing.T) {
	t.Parallel()

	err := queryAnswering(t, http.StatusBadRequest,
		`{"type":"BadRequest","message":"Invalid field nosuchfield","errors":[{"name":"nosuchfield"}]}`)
	assert.NotContains(t, err.Error(), "json: ", "the client decodes TheHive's 400 body")
	assert.NotContains(t, err.Error(), "unknown field")

	var apiErr *thehive.GenericOpenAPIError
	require.ErrorAs(t, err, &apiErr)
	assert.NotNil(t, apiErr.Model(), "the decoded error model is attached")
	assert.Same(t, err, StatusCause(err, badRequest), "a decoded error is the cause as it is")
}

func TestStatusCauseKeepsOtherErrors(t *testing.T) {
	t.Parallel()

	decoded := queryAnswering(t, http.StatusBadRequest, `{"type":"BadRequest","message":"Invalid filter"}`)
	assert.Same(t, decoded, StatusCause(decoded, badRequest), "a body the client decoded keeps its error")

	// A 2xx the client cannot decode is a schema drift: its message names what
	// failed, and its body is entity data.
	drift := queryAnswering(t, http.StatusOK, `{"_id":"~1","title":"secret case"`)
	assert.Same(t, drift, StatusCause(drift, ok200))

	plain := fmt.Errorf("request to TheHive: %w", context.DeadlineExceeded)
	assert.Same(t, plain, StatusCause(plain, badRequest))

	failure := queryAnswering(t, http.StatusBadRequest, `{"type":"BadRequest","message":"x","errors":[]}`)
	wrapped := fmt.Errorf("failed to search cases: %w", failure)
	assert.Same(t, wrapped, StatusCause(wrapped, badRequest), "a caller's message around the client's error is kept")
	assert.Same(t, failure, StatusCause(failure, nil), "no response, nothing to point at")
	assert.Same(t, failure, StatusCause(failure, ok200), "a 2xx body is never pointed at as an error")
}

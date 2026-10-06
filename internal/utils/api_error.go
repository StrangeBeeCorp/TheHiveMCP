package utils

import (
	"net/http"
	"strings"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

// statusError is a thehive4go error that reads as TheHive's status line.
type statusError struct {
	text string
	err  error
}

func (e *statusError) Error() string { return e.text }
func (e *statusError) Unwrap() error { return e.err }

// StatusCause returns err, unless it is thehive4go failing to decode TheHive's
// error body: an error answer holding a field the client's model does not
// declare (a 400's "errors") makes the client report its own decoding error,
// `json: unknown field "errors"`, ahead of what TheHive said. The returned error
// then reads as TheHive's status line and points at the body, which the caller
// shows where it already did (the API response). It still unwraps to err.
//
// Only an error status is rewritten: a 2xx the client cannot decode is a schema
// drift worth naming, and its body is entity data. Only err itself is looked
// at, not what it wraps: a caller's message around the client's error carries
// guidance this must not replace.
func StatusCause(err error, resp *http.Response) error {
	apiErr, isAPIErr := err.(*thehive.GenericOpenAPIError) //nolint:errorlint // err itself, by design (see above)
	if !isAPIErr || apiErr.Model() != nil || !strings.HasPrefix(apiErr.Error(), "json: ") ||
		resp == nil || resp.StatusCode < http.StatusBadRequest {
		return err
	}

	return &statusError{text: "TheHive answered " + resp.Status + ": see its message in the API response", err: err}
}

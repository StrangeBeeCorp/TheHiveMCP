package utils

import (
	"strings"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

// decodedAPIError is a thehive4go error whose text is TheHive's own answer.
type decodedAPIError struct {
	text string
	err  error
}

func (e *decodedAPIError) Error() string { return e.text }
func (e *decodedAPIError) Unwrap() error { return e.err }

// ReadableAPIError returns err, unless it is thehive4go failing to decode
// TheHive's answer: a body its model does not declare (an error's "errors"
// field, a 207's {success, failure}) makes the client report its own decoding
// error, `json: unknown field "errors"`, which hides what TheHive said. The
// returned error then reads as TheHive's body (bounded), and still unwraps to
// err for errors.Is and errors.As.
//
// Only err itself is looked at, not what it wraps: a caller's message around
// the client's error carries guidance this must not replace.
func ReadableAPIError(err error) error {
	apiErr, isAPIErr := err.(*thehive.GenericOpenAPIError) //nolint:errorlint // err itself, by design (see above)
	if !isAPIErr || apiErr.Model() != nil || !strings.HasPrefix(apiErr.Error(), "json: ") {
		return err
	}

	body := strings.TrimSpace(string(apiErr.Body()))
	if body == "" {
		return err
	}

	if len(body) > maxDescribedBodyBytes {
		body = body[:maxDescribedBodyBytes] + "…"
	}

	return &decodedAPIError{text: "TheHive answered: " + body, err: err}
}

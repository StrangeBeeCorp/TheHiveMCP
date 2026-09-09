package utils

import (
	"io"
	"net/http"
	"strconv"
	"strings"
)

// maxDescribedBodyBytes bounds how much of an error body reaches the caller, so
// a large HTML error page cannot flood the model's context.
const maxDescribedBodyBytes = 2048

// DescribeHTTPResponse renders the part of an API response worth showing a
// caller: the status line, and whatever the body still holds.
//
// It exists because `fmt.Errorf("API response: %v", resp)` formats the whole
// *http.Response struct — pointer addresses, the transport's headers, the
// server's identity and version. None of that helps the caller diagnose a bad
// field name, all of it is noise in the model's context, and the header block
// leaks deployment detail into an error a client may surface verbatim.
//
// The body is usually empty here: thehive4go reads it while building its own
// error, and a body can only be read once. That is fine — the status line alone
// beats a struct dump, and thehive4go's error (carried separately as the cause)
// already holds what it managed to parse.
func DescribeHTTPResponse(resp *http.Response) string {
	if resp == nil {
		return "no response"
	}

	status := resp.Status
	if status == "" {
		status = strconv.Itoa(resp.StatusCode)
	}

	if resp.Body == nil {
		return status
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDescribedBodyBytes))
	_ = resp.Body.Close()

	if err != nil || len(body) == 0 {
		return status
	}

	return status + ": " + strings.TrimSpace(string(body))
}

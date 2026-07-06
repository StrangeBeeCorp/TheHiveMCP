package manage

import "net/http"

// closeResponse drains and closes an HTTP response body if the response is non-nil.
// It is safe to defer immediately after a call whose response may be nil (e.g. on error).
func closeResponse(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

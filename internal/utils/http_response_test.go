package utils

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A 4xx other than 429 will not change on a retry; a 429, a 5xx or no response
// at all might.
func TestIsClientError(t *testing.T) {
	t.Parallel()

	assert.False(t, IsClientError(nil))

	for code, want := range map[int]bool{
		http.StatusBadRequest:          true,
		http.StatusForbidden:           true,
		http.StatusNotFound:            true,
		http.StatusTooManyRequests:     false,
		http.StatusInternalServerError: false,
		http.StatusBadGateway:          false,
		http.StatusOK:                  false,
	} {
		assert.Equal(t, want, IsClientError(&http.Response{StatusCode: code}), "status %d", code)
	}
}

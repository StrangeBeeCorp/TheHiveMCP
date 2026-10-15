package testutils

import (
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

// FakeQuery answers every POST /api/v1/query with one raw reply, so unit tests
// can feed search-entities rows exactly as a TheHive sent them (the 5.8 fields
// the client's models do not list included) without a container. Like
// FakeCortex, it keeps net/http out of the tests that use it.
type FakeQuery struct {
	// Reply is the body TheHive answers, a recorded one (RecordedAnswer) or a bare
	// number for a count. Its status is 200 unless it says otherwise.
	Reply Answer

	queries atomic.Int32
}

// Client starts the fake and returns a TheHive client pointed at it.
func (f *FakeQuery) Client(t *testing.T) *thehive.APIClient {
	t.Helper()

	return newFakeClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)

			return
		}

		f.queries.Add(1)
		f.Reply.write(w)
	}))
}

// Queries reports how many queries the fake has answered.
func (f *FakeQuery) Queries() int {
	return int(f.queries.Load())
}

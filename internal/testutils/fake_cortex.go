package testutils

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

// FakeCortex serves TheHive's analyzer and responder catalogs from memory, so
// unit tests can exercise code that reads them without a TheHive container.
// Tests using it need not import net/http, which would make bodyclose flag
// every SDK call in their package.
type FakeCortex struct {
	Analyzers  []thehive.OutputWorker
	Responders []thehive.OutputWorker // served for every entity
	Down       bool                   // when set, every request fails with 503

	requests atomic.Int32
}

// Client starts the fake and returns a TheHive client pointed at it.
func (f *FakeCortex) Client(t *testing.T) *thehive.APIClient {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)

	cfg := thehive.NewConfiguration()
	cfg.Servers = thehive.ServerConfigurations{{URL: server.URL}}

	return thehive.NewAPIClient(cfg)
}

// Requests reports how many requests the fake has answered.
func (f *FakeCortex) Requests() int {
	return int(f.requests.Load())
}

func (f *FakeCortex) serve(w http.ResponseWriter, r *http.Request) {
	f.requests.Add(1)

	if f.Down {
		w.WriteHeader(http.StatusServiceUnavailable)

		return
	}

	var workers []thehive.OutputWorker

	switch {
	case r.URL.Path == "/api/v1/connector/cortex/analyzer":
		workers = f.Analyzers
	case strings.HasPrefix(r.URL.Path, "/api/v1/connector/cortex/responder/"):
		workers = f.Responders
	default:
		w.WriteHeader(http.StatusNotFound)

		return
	}

	body, err := json.Marshal(workers)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

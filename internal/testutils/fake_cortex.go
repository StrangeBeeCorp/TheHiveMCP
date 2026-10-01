package testutils

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
	Responders []thehive.OutputWorker // served for every entity TheHive knows
	Down       bool                   // when set, every request fails with 503

	requests atomic.Int32

	mu          sync.Mutex
	actionTypes []string
}

// responderEntityTypes are the entity types TheHive's responder and action
// endpoints resolve (v0 Conversion.toObjectType); any other one answers 404.
var responderEntityTypes = map[string]bool{"case": true, "alert": true, "case_task": true, "case_artifact": true, "case_task_log": true}

// ActionObjectTypes lists the objectType of every action created, in order.
func (f *FakeCortex) ActionObjectTypes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.actionTypes...)
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
		entityType, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/api/v1/connector/cortex/responder/"), "/")
		if !responderEntityTypes[entityType] {
			w.WriteHeader(http.StatusNotFound)

			return
		}

		workers = f.Responders
	case r.URL.Path == "/api/v1/connector/cortex/action" && r.Method == http.MethodPost:
		f.createAction(w, r)

		return
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

// createAction records the action's objectType and answers it the way TheHive
// does, with 404 for an entity type it does not resolve.
func (f *FakeCortex) createAction(w http.ResponseWriter, r *http.Request) {
	var input thehive.InputAction

	err := json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	f.mu.Lock()
	f.actionTypes = append(f.actionTypes, input.ObjectType)
	f.mu.Unlock()

	if !responderEntityTypes[input.ObjectType] {
		w.WriteHeader(http.StatusNotFound)

		return
	}

	body, err := json.Marshal(thehive.OutputAction{
		UnderscoreId: "~action-1", UnderscoreType: "Action", UnderscoreCreatedBy: "test", UnderscoreCreatedAt: 1,
		ResponderId: input.ResponderId, ObjectType: input.ObjectType, ObjectId: input.ObjectId,
		Status: "Waiting", StartDate: 1, Operations: "[]", Report: "{}",
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

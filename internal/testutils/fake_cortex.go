package testutils

import (
	"embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

// recorded holds answers captured from a real TheHive (see RecordedAnswer).
//
//go:embed testdata/recorded/*.json
var recorded embed.FS

const recordedDir = "testdata/recorded/"

// RecordedAnswer returns the body of a recorded TheHive answer, by file name
// under testdata/recorded. Fakes answer with these, never with the client's own
// models: a model marshalled by the client always decodes back, so it can never
// show that the client does not know a field TheHive sends.
func RecordedAnswer(t *testing.T, name string) string {
	t.Helper()

	body, err := recorded.ReadFile(recordedDir + name)
	if err != nil {
		t.Fatalf("no recorded TheHive answer %q: %v", name, err)
	}

	return string(body)
}

// Answer is a raw reply of a fake: a status (200 when zero) and a body, sent
// as they are. The zero Answer means "no override": the fake gives its recorded
// answer.
type Answer struct {
	Status int
	Body   string
}

func (a Answer) isSet() bool { return a.Status != 0 || a.Body != "" }

// write sends the answer as JSON.
func (a Answer) write(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")

	if a.Status != 0 {
		w.WriteHeader(a.Status)
	}

	_, _ = w.Write([]byte(a.Body))
}

// writeRecorded sends a recorded answer, or 500 when the file is missing.
func writeRecorded(w http.ResponseWriter, name string) {
	body, err := recorded.ReadFile(recordedDir + name)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// newFakeClient starts handler as a TheHive and returns a client pointed at it.
func newFakeClient(t *testing.T, handler http.Handler) *thehive.APIClient {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	cfg := thehive.NewConfiguration()
	cfg.Servers = thehive.ServerConfigurations{{URL: server.URL}}

	return thehive.NewAPIClient(cfg)
}

// FakeCortex serves TheHive's analyzer and responder catalogs from memory, so
// unit tests can exercise code that reads them without a TheHive container.
// Tests using it need not import net/http, which would make bodyclose flag
// every SDK call in their package.
//
// Its answers for actions and custom fields are TheHive's recorded ones
// (testdata/recorded), the 5.8 shape included. An Answer field replaces one
// with a raw reply, to play a drifted or failing TheHive.
type FakeCortex struct {
	Analyzers  []thehive.OutputWorker
	Responders []thehive.OutputWorker // served for every entity TheHive knows
	Down       bool                   // when set, every request fails with 503

	ActionAnswer       Answer // POST /api/v1/connector/cortex/action (run-responder)
	JobAnswer          Answer // POST /api/v1/connector/cortex/job (run-analyzer); 404 when unset
	CustomFieldsAnswer Answer // GET /api/v1/customField

	requests atomic.Int32

	mu      sync.Mutex
	actions []thehive.InputAction
}

// responderEntityTypes are the entity types TheHive's responder and action
// endpoints resolve (v0 Conversion.toObjectType); any other one answers 404.
var responderEntityTypes = map[string]bool{"case": true, "alert": true, objectTypeCaseTask: true, "case_artifact": true, "case_task_log": true}

// ActionObjectTypes lists the objectType of every action created, in order.
func (f *FakeCortex) ActionObjectTypes() []string {
	actions := f.Actions()

	objectTypes := make([]string, 0, len(actions))
	for _, action := range actions {
		objectTypes = append(objectTypes, action.ObjectType)
	}

	return objectTypes
}

// Actions lists every action created, as TheHive received it, in order.
func (f *FakeCortex) Actions() []thehive.InputAction {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]thehive.InputAction(nil), f.actions...)
}

// Client starts the fake and returns a TheHive client pointed at it.
func (f *FakeCortex) Client(t *testing.T) *thehive.APIClient {
	t.Helper()

	return newFakeClient(t, http.HandlerFunc(f.serve))
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
	case r.URL.Path == "/api/v1/connector/cortex/job" && r.Method == http.MethodPost && f.JobAnswer.isSet():
		f.JobAnswer.write(w)

		return
	case strings.HasPrefix(r.URL.Path, "/api/v1/connector/cortex/action/") && r.Method == http.MethodGet:
		f.listActions(w, r)

		return
	case r.URL.Path == "/api/v1/customField" && r.Method == http.MethodGet:
		f.listCustomFields(w)

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

// objectTypeCaseTask is the entity type TheHive's responder endpoints give a task.
const objectTypeCaseTask = "case_task"

// recordedTaskID is the task the recorded actions ran on.
const recordedTaskID = "~2703511728"

// createAction records the action and answers it the way TheHive does: with
// the recorded answer for a case or a task, and 404 for an entity type it does
// not resolve.
func (f *FakeCortex) createAction(w http.ResponseWriter, r *http.Request) {
	var input thehive.InputAction

	err := json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	f.mu.Lock()
	f.actions = append(f.actions, input)
	f.mu.Unlock()

	if f.ActionAnswer.isSet() {
		f.ActionAnswer.write(w)

		return
	}

	if !responderEntityTypes[input.ObjectType] {
		w.WriteHeader(http.StatusNotFound)

		return
	}

	if input.ObjectType == objectTypeCaseTask {
		writeRecorded(w, "action-created-task.json")

		return
	}

	writeRecorded(w, "action-created-case.json")
}

// listActions answers the actions of an entity: the recorded one for the task
// they ran on, none for any other entity TheHive resolves.
func (f *FakeCortex) listActions(w http.ResponseWriter, r *http.Request) {
	entityType, entityID, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/api/v1/connector/cortex/action/"), "/")
	if !responderEntityTypes[entityType] {
		w.WriteHeader(http.StatusNotFound)

		return
	}

	if entityType == objectTypeCaseTask && entityID == recordedTaskID {
		writeRecorded(w, "action-list-task.json")

		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte("[]"))
}

// listCustomFields answers the recorded custom fields: every kind of options
// TheHive keeps, an empty list and integer options included.
func (f *FakeCortex) listCustomFields(w http.ResponseWriter) {
	if f.CustomFieldsAnswer.isSet() {
		f.CustomFieldsAnswer.write(w)

		return
	}

	writeRecorded(w, "custom-fields.json")
}

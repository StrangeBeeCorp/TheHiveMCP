package testutils

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCortexStack plays a Cortex that starts with no database, and the TheHive whose connector
// the bootstrap sets. It records every request, with its basic-auth user.
type fakeCortexStack struct {
	mu sync.Mutex
	// stage: 0 no database, 1 no user, 2 users exist.
	stage int
	// listed is how many definition listings come back empty before the neurons show up.
	listed int
	// existing makes every create answer ConflictError, as on a rerun.
	existing bool
	// failOn answers 500 to that path.
	failOn string
	calls  []string
	// connector is the body TheHive received.
	connector *theHiveConnectorBody
}

func (f *fakeCortexStack) record(r *http.Request) {
	user, _, _ := r.BasicAuth()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path+" "+user)
}

func (f *fakeCortexStack) cortex(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.record(r)

	if r.URL.Path == f.failOn {
		http.Error(w, `{"type":"InternalError","message":"boom"}`, http.StatusInternalServerError)
		return
	}

	switch {
	case r.URL.Path == "/api/user/current":
		w.WriteHeader([]int{cortexNoDatabase, http.StatusNotFound, http.StatusUnauthorized}[f.stage])
	case r.URL.Path == "/api/maintenance/migrate":
		f.stage = max(f.stage, 1)

		w.WriteHeader(http.StatusNoContent)
	case r.URL.Path == "/api/user" && f.stage == 1:
		f.stage = 2

		w.WriteHeader(http.StatusCreated)
	case strings.HasSuffix(r.URL.Path, "definition"):
		if f.listed > 0 {
			f.listed--
			_, _ = io.WriteString(w, "[]")

			return
		}

		_, _ = io.WriteString(w, `[{"id":"`+TestCortexAnalyzer+`"},{"id":"`+TestCortexResponder+`"}]`)
	case strings.HasSuffix(r.URL.Path, "/password/set"):
		w.WriteHeader(http.StatusNoContent)
	case f.existing:
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"type":"ConflictError","message":"version conflict, document already exists"}`)
	default:
		w.WriteHeader(http.StatusCreated)
	}
}

func (f *fakeCortexStack) theHive(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.record(r)

	if r.URL.Path == f.failOn {
		http.Error(w, `{"type":"AuthorizationError"}`, http.StatusForbidden)
		return
	}

	f.connector = &theHiveConnectorBody{}
	_ = json.NewDecoder(r.Body).Decode(f.connector)

	w.WriteHeader(http.StatusNoContent)
}

// wiring runs the bootstrap against the fakes, with a short poll.
func (f *fakeCortexStack) wire(t *testing.T) error {
	t.Helper()

	cortex := httptest.NewServer(http.HandlerFunc(f.cortex))
	t.Cleanup(cortex.Close)

	theHive := httptest.NewServer(http.HandlerFunc(f.theHive))
	t.Cleanup(theHive.Close)

	w := cortexWiring{
		cortex: cortex.URL, theHive: theHive.URL, theHiveUser: "admin@thehive.local", theHivePassword: "secret",
		client: cortex.Client(), ready: 5 * time.Second, poll: time.Millisecond,
	}

	return w.wire(context.Background())
}

func TestCortexWiringBootstrapsAFreshCortexThenSetsTheConnector(t *testing.T) {
	t.Parallel()

	stack := &fakeCortexStack{listed: 2}
	require.NoError(t, stack.wire(t))

	// probe is the anonymous request Cortex's state shows in; listAnalyzers waits for the catalog.
	const (
		probe         = "GET /api/user/current "
		listAnalyzers = "GET /api/analyzerdefinition thehive"
	)

	assert.Equal(t, []string{
		probe, "POST /api/maintenance/migrate ",
		probe, "POST /api/user ",
		probe,
		"POST /api/organization admin", "POST /api/user admin", "POST /api/user/thehive/password/set admin",
		listAnalyzers, listAnalyzers, listAnalyzers,
		"POST /api/organization/analyzer/" + TestCortexAnalyzer + " thehive",
		"GET /api/responderdefinition thehive", "POST /api/organization/responder/" + TestCortexResponder + " thehive",
		"PUT /api/config/cortex.servers admin@thehive.local",
	}, stack.calls)

	assert.Equal(t, &theHiveConnectorBody{Value: []theHiveCortexServer{{
		Name: "local", URL: "http://cortex:9001", Auth: theHiveAuthBody{Type: "basic", Username: "thehive", Password: "secret"},
	}}}, stack.connector, "TheHive calls the compose service as the organisation's user")
}

func TestCortexWiringTakesWhatARerunFindsAsDone(t *testing.T) {
	t.Parallel()

	stack := &fakeCortexStack{stage: 2, existing: true}
	require.NoError(t, stack.wire(t), "ConflictError is the answer to a create of what an earlier run made")
	assert.NotNil(t, stack.connector)
}

func TestCortexWiringStopsAtTheStepCortexRefuses(t *testing.T) {
	t.Parallel()

	stack := &fakeCortexStack{stage: 2, failOn: "/api/organization"}
	err := stack.wire(t)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "create its organisation and user")
	assert.Contains(t, err.Error(), "answered 500")
	assert.Nil(t, stack.connector, "TheHive is not pointed at a Cortex that is not ready")
}

func TestCortexWiringReportsWhenTheHiveRefusesTheConnector(t *testing.T) {
	t.Parallel()

	stack := &fakeCortexStack{stage: 2, failOn: "/api/config/cortex.servers"}
	err := stack.wire(t)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "set TheHive's Cortex connector")
	assert.Contains(t, err.Error(), "answered 403")
}

func TestCortexWiringGivesUpOnACortexThatNeverAnswers(t *testing.T) {
	t.Parallel()

	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()

	w := cortexWiring{cortex: gone.URL, theHive: gone.URL, client: http.DefaultClient, ready: 50 * time.Millisecond, poll: 5 * time.Millisecond}
	err := w.wire(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not ready in time")
}

func TestCortexWiringGivesUpWhenTheNeuronsNeverShow(t *testing.T) {
	t.Parallel()

	cortex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/user/current" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		_, _ = io.WriteString(w, "[]")
	}))
	t.Cleanup(cortex.Close)

	w := cortexWiring{cortex: cortex.URL, theHive: cortex.URL, client: cortex.Client(), ready: time.Second, poll: 10 * time.Millisecond}
	err := w.wire(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no analyzer definition "+TestCortexAnalyzer)
}

func TestCortexBesidesTheHiveIsPort9001OfItsHost(t *testing.T) {
	t.Parallel()

	for theHive, cortex := range map[string]string{
		"http://localhost:9000":                "http://localhost:9001",
		"http://thehive.example:19000/":        "http://thehive.example:9001",
		"https://[::1]:9000":                   "http://[::1]:9001",
		"not a url with a host":                "http://localhost:9001",
		"http://thehive.devstack.orb.local:80": "http://thehive.devstack.orb.local:9001",
	} {
		assert.Equal(t, cortex, cortexBesides(theHive), theHive)
	}
}

func TestCortexTestURLTakesTheOverride(t *testing.T) {
	t.Setenv(envTestCortexURL, "http://cortex.example:19001")
	assert.Equal(t, "http://cortex.example:19001", CortexTestURL())

	t.Setenv(envTestCortexURL, "")
	t.Setenv("THEHIVE_TEST_URL", "http://stack.example:9000")
	assert.Equal(t, "http://stack.example:9001", CortexTestURL())
}

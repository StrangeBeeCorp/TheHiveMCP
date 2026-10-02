package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createdObservable is what TheHive answers to an observable create that made one.
const createdObservable = `[{"_id":"~8192","_type":"Observable","_createdBy":"analyst@example.com","_createdAt":1767225600000,` +
	`"dataType":"ip","data":"203.0.113.9","startDate":1767225600000,"tlp":2,"tlpLabel":"AMBER","pap":2,"papLabel":"AMBER",` +
	`"ioc":false,"sighted":false,"ignoreSimilarity":false,"reports":{},"extraData":{},"tags":[]}]`

// observableStub is a TheHive that answers an observable create in case ~1 with answer, and records
// every other request it gets.
func observableStub(t *testing.T, answer string) (*httptest.Server, *[]string) {
	t.Helper()

	var (
		mu    sync.Mutex
		other []string
	)

	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/case/~1/observable" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(answer))

			return
		}

		mu.Lock()

		other = append(other, r.Method+" "+r.URL.Path)

		mu.Unlock()
		http.NotFound(w, r)
	}))
	t.Cleanup(stub.Close)

	return stub, &other
}

func createObservable(t *testing.T, stub *httptest.Server) string {
	t.Helper()

	result := callTool(t, stub.URL, filepath.Join("..", "docs", "reference", "permissions-examples", "analyst.yaml"),
		"manage-entities", map[string]any{
			"operation": "create", "entity-type": "observable", "entity-ids": []string{"~1"},
			"entity-data": map[string]any{"dataType": "ip", "data": "203.0.113.9"},
		})

	if result.IsError {
		return "error: " + resultText(result)
	}

	return resultText(result)
}

// TheHive answers 2xx with an empty list when an observable create made nothing: the tool says so,
// with a hint, instead of reporting a success. The same call that makes an observable succeeds.
//
// spec: SIL-001
func TestACreateTheHiveAnswersWithNothingIsAnError(t *testing.T) {
	t.Parallel()

	stub, other := observableStub(t, "[]")
	got := createObservable(t, stub)
	assert.Contains(t, got, "error: ", "a 2xx that created nothing must be an error")
	assert.Contains(t, got, "TheHive created no observable")
	assert.Contains(t, got, "entity-data.data", "the hint says where the value goes")
	assert.Empty(t, *other, "only the create reached TheHive")

	stub, _ = observableStub(t, createdObservable)
	got = createObservable(t, stub)
	require.NotContains(t, got, "error: ", "a create that made an observable succeeds")
	assert.Contains(t, got, "~8192")
}

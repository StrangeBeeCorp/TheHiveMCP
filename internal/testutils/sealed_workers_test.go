package testutils

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sealedServe(t *testing.T, cortex *FakeCortex, path string) []map[string]any {
	t.Helper()

	base := cortex.Client(t).GetConfig().Servers[0].URL

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, base+path, http.NoBody)
	require.NoError(t, err)

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)

	defer func() { _ = response.Body.Close() }()

	require.Equal(t, http.StatusOK, response.StatusCode)

	raw, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	var entries []map[string]any

	require.NoError(t, json.Unmarshal(raw, &entries))

	return entries
}

func sealedCatalog(t *testing.T, file string) []map[string]any {
	t.Helper()

	var catalog []map[string]any

	require.NoError(t, json.Unmarshal([]byte(RecordedAnswer(t, file)), &catalog))
	require.NotEmpty(t, catalog)

	return catalog
}

// Every recorded entry round-trips through the fake untouched, whatever its
// name: nothing is special-cased for the two or three workers the visible tests use.
//
// spec: VER-002
func TestEveryRecordedWorkerIsServedUntouched(t *testing.T) {
	t.Parallel()

	analyzers := sealedCatalog(t, "analyzer-catalog.json")
	responders := sealedCatalog(t, "responder-catalog-case.json")

	cortex := &FakeCortex{}

	for _, entry := range analyzers {
		name, ok := entry["name"].(string)
		require.True(t, ok)

		cortex.Analyzers = append(cortex.Analyzers, RecordedAnalyzer(t, name))
	}

	for _, entry := range responders {
		name, ok := entry["name"].(string)
		require.True(t, ok)

		cortex.Responders = append(cortex.Responders, RecordedResponder(t, name))
	}

	assert.Equal(t, analyzers, sealedServe(t, cortex, "/api/v1/connector/cortex/analyzer?range=all"))
	assert.Equal(t, responders, sealedServe(t, cortex, "/api/v1/connector/cortex/responder/case/~1"))
}

// An override builds a new worker: it changes neither the one it started from
// nor the recorded entry the next test takes.
//
// spec: VER-002
func TestOverridingAWorkerLeavesTheRecordedEntryAlone(t *testing.T) {
	t.Parallel()

	original := RecordedAnalyzer(t, "Shodan_Host_1_0")
	originalID := original.ID()

	changed := original.With("name", "Other_1_0").With("id", "zzz").With("dataTypeList", []string{"mail"})

	assert.Equal(t, "Shodan_Host_1_0", original.Name(), "the receiver is not changed")
	assert.Equal(t, originalID, original.ID())
	assert.Equal(t, "Other_1_0", changed.Name())
	assert.Equal(t, "zzz", changed.ID())

	again := RecordedAnalyzer(t, "Shodan_Host_1_0")
	assert.Equal(t, "Shodan_Host_1_0", again.Name(), "the next test gets the recorded entry")
	assert.Equal(t, originalID, again.ID())

	served := sealedServe(t, &FakeCortex{Analyzers: []Worker{again}}, "/api/v1/connector/cortex/analyzer")
	require.Len(t, served, 1)
	assert.Equal(t, sealedCatalog(t, "analyzer-catalog.json")[servedIndex(t, "Shodan_Host_1_0")], served[0])
}

func servedIndex(t *testing.T, name string) int {
	t.Helper()

	for i, entry := range sealedCatalog(t, "analyzer-catalog.json") {
		if entry["name"] == name {
			return i
		}
	}

	require.FailNow(t, "not recorded", name)

	return -1
}

// Two instances of one worker (same name, another ID) are two entries of the
// catalog, in the order the test gave them.
func TestTwoWorkersOfTheSameNameAreBothServedInOrder(t *testing.T) {
	t.Parallel()

	first := RecordedResponder(t, "Mailer_1_0")
	second := first.With("id", "e9f8a7")

	served := sealedServe(t, &FakeCortex{Responders: []Worker{second, first}}, "/api/v1/connector/cortex/responder/case_task/~7")

	require.Len(t, served, 2)
	assert.Equal(t, "e9f8a7", served[0]["id"])
	assert.Equal(t, first.ID(), served[1]["id"])
	assert.Equal(t, served[0]["name"], served[1]["name"])
}

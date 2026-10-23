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

// getBody reads what a fake answers on path, raw: the test looks at the JSON
// TheHive would have sent, not at what the client made of it.
func getBody(t *testing.T, cortex *FakeCortex, path string) []byte {
	t.Helper()

	base := cortex.Client(t).GetConfig().Servers[0].URL

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, base+path, http.NoBody)
	require.NoError(t, err)

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)

	defer func() { _ = response.Body.Close() }()

	require.Equal(t, http.StatusOK, response.StatusCode, path)

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	return body
}

// recordedEntry is one entry of a recorded catalog, by name.
func recordedEntry(t *testing.T, file, name string) map[string]any {
	t.Helper()

	var catalog []map[string]any

	require.NoError(t, json.Unmarshal([]byte(RecordedAnswer(t, file)), &catalog))

	for _, entry := range catalog {
		if entry["name"] == name {
			return entry
		}
	}

	require.FailNow(t, "the recorded catalog has no such worker", "%s in %s", name, file)

	return nil
}

func asJSON(t *testing.T, value any) string {
	t.Helper()

	encoded, err := json.Marshal(value)
	require.NoError(t, err)

	return string(encoded)
}

// The catalogs are TheHive's recorded entries, as they were sent: the fields
// the client does not model (cortexIds is one TheHive 6 keeps) stay.
//
// spec: VER-002
func TestFakeCortexServesTheAnalyzersItWasGivenAsTheyWereRecorded(t *testing.T) {
	t.Parallel()

	cortex := &FakeCortex{Analyzers: []Worker{
		RecordedAnalyzer(t, "AbuseIPDB_2_0"),
		RecordedAnalyzer(t, "EmailRep_1_0"),
	}}

	body := getBody(t, cortex, "/api/v1/connector/cortex/analyzer?range=all")

	want := []map[string]any{
		recordedEntry(t, "analyzer-catalog.json", "AbuseIPDB_2_0"),
		recordedEntry(t, "analyzer-catalog.json", "EmailRep_1_0"),
	}
	assert.JSONEq(t, asJSON(t, want), string(body))
}

// spec: VER-002
func TestFakeCortexServesTheRespondersItWasGivenAsTheyWereRecorded(t *testing.T) {
	t.Parallel()

	cortex := &FakeCortex{Responders: []Worker{RecordedResponder(t, "Mailer_1_0")}}

	body := getBody(t, cortex, "/api/v1/connector/cortex/responder/case/~1")

	want := []map[string]any{recordedEntry(t, "responder-catalog-case.json", "Mailer_1_0")}
	assert.JSONEq(t, asJSON(t, want), string(body))
}

// A test that needs a worker of its own takes a recorded entry and overrides
// the fields it cares about; every other field is the recorded one.
//
// spec: VER-002
func TestAWorkerOverridesOnlyTheFieldsTheTestNames(t *testing.T) {
	t.Parallel()

	worker := RecordedResponder(t, "Mailer_1_0").
		With("id", "e9f8a7").
		With("name", "Renamed_1_0").
		With("dataTypeList", []string{"thehive:case"})

	assert.Equal(t, "e9f8a7", worker.ID())
	assert.Equal(t, "Renamed_1_0", worker.Name())

	want := recordedEntry(t, "responder-catalog-case.json", "Mailer_1_0")
	want["id"] = "e9f8a7"
	want["name"] = "Renamed_1_0"
	want["dataTypeList"] = []string{"thehive:case"}

	body := getBody(t, &FakeCortex{Responders: []Worker{worker}}, "/api/v1/connector/cortex/responder/case/~1")
	assert.JSONEq(t, asJSON(t, []map[string]any{want}), string(body))
}

func TestARecordedWorkerKnowsItsIDAndName(t *testing.T) {
	t.Parallel()

	recorded := recordedEntry(t, "analyzer-catalog.json", "VirusTotal_GetReport_3_1")
	worker := RecordedAnalyzer(t, "VirusTotal_GetReport_3_1")

	assert.Equal(t, recorded["id"], worker.ID())
	assert.Equal(t, "VirusTotal_GetReport_3_1", worker.Name())
}

// TheHive answers an empty list, not null, when Cortex knows no worker.
//
// spec: VER-002
func TestFakeCortexWithoutWorkersAnswersAnEmptyCatalog(t *testing.T) {
	t.Parallel()

	cortex := &FakeCortex{}

	assert.JSONEq(t, "[]", string(getBody(t, cortex, "/api/v1/connector/cortex/analyzer?range=all")))
	assert.JSONEq(t, "[]", string(getBody(t, cortex, "/api/v1/connector/cortex/responder/case/~1")))
}

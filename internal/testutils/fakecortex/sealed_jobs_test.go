package fakecortex_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils/fakecortex"
)

func sealedRead(t *testing.T, fake *fakecortex.Jobs, jobID string) map[string]any {
	t.Helper()

	base := fake.Client(t).GetConfig().Servers[0].URL

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, base+"/api/v1/connector/cortex/job/"+jobID, http.NoBody)
	require.NoError(t, err)

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)

	defer func() { _ = response.Body.Close() }()

	require.Equal(t, http.StatusOK, response.StatusCode)

	raw, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	var job map[string]any

	require.NoError(t, json.Unmarshal(raw, &job))

	return job
}

// Each job carries its own identifier and the status of its own reply, and the
// replies of one job play in turn: nothing is shared between jobs or reads.
//
// spec: VER-002
func TestEachJobIsAnsweredWithItsOwnIdentifierAndReplies(t *testing.T) {
	t.Parallel()

	fake := fakecortex.NewJobs(map[string][]fakecortex.Reply{
		"~11": {fakecortex.Status("Waiting"), fakecortex.Status("InProgress"), fakecortex.Status("Failure")},
		"~22": {fakecortex.Status("Success")},
	})

	for _, status := range []string{"Waiting", "InProgress", "Failure", "Failure"} {
		job := sealedRead(t, fake, "~11")

		assert.Equal(t, "~11", job["_id"])
		assert.Equal(t, "~11", job["id"])
		assert.Equal(t, status, job["status"])
	}

	other := sealedRead(t, fake, "~22")
	assert.Equal(t, "~22", other["_id"])
	assert.Equal(t, "~22", other["id"])
	assert.Equal(t, "Success", other["status"])
	assert.Equal(t, "case_artifact_job", other["_type"])
}

// A finished job carries the report a wait or a status read decodes, as an
// object with its verdict, and the recorded analyzer it ran.
//
// spec: VER-002
func TestAFinishedJobCarriesAReportObjectAndTheRecordedAnalyzer(t *testing.T) {
	t.Parallel()

	job := sealedRead(t, fakecortex.NewJobs(map[string][]fakecortex.Reply{"~5": {fakecortex.Status("Success")}}), "~5")

	report, ok := job["report"].(map[string]any)
	require.True(t, ok, "the report is an object: %v", job["report"])
	assert.Contains(t, report, "success")
	assert.Equal(t, "Crt_sh_Transparency_Logs_1_0", job["analyzerName"])
	assert.Equal(t, "7ed131a7c3f1fb19caf49295621327dc", job["analyzerId"])
	assert.Equal(t, "[]", job["operations"], "operations is the string TheHive sends")
}

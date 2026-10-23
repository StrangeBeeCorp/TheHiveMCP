package fakecortex_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils/fakecortex"
)

// recordedJob is the job TheHive 6.0 answered once Cortex had finished it.
func recordedJob(t *testing.T) map[string]any {
	t.Helper()

	raw, err := os.ReadFile("../testdata/recorded/job-finished.json")
	require.NoError(t, err)

	var job map[string]any

	require.NoError(t, json.Unmarshal(raw, &job))

	return job
}

// readJob reads a job from the fake as raw JSON: the test looks at what
// TheHive would have sent, not at what the client made of it.
func readJob(t *testing.T, fake *fakecortex.Jobs, jobID string) map[string]any {
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

// The job is the recorded one, with the fields the fake plays overridden: its
// identifier, its status and the report a test reads. Every other field is
// what TheHive sent, a _type the client's model has no value for included.
//
// spec: VER-002
func TestJobFakeAnswersWithTheRecordedJobAndOverridesOnlyWhatItPlays(t *testing.T) {
	t.Parallel()

	fake := fakecortex.NewJobs(map[string][]fakecortex.Reply{"~7": {fakecortex.Status("Success")}})

	job := readJob(t, fake, "~7")
	recorded := recordedJob(t)

	assert.Equal(t, "~7", job["_id"])
	assert.Equal(t, "~7", job["id"])
	assert.Equal(t, "Success", job["status"])

	played := map[string]bool{"_id": true, "id": true, "status": true, "report": true, "cortexJobId": true}

	for field, want := range recorded {
		if played[field] {
			require.Contains(t, job, field)

			continue
		}

		assert.Equal(t, want, job[field], field)
	}

	for field := range job {
		assert.Contains(t, recorded, field, "the fake invents no field TheHive did not send")
	}

	assert.Equal(t, "case_artifact_job", job["_type"])
}

func TestJobFakeAnswersTheStatusOfEachReply(t *testing.T) {
	t.Parallel()

	fake := fakecortex.NewJobs(map[string][]fakecortex.Reply{"~1": {fakecortex.Status("InProgress")}})

	assert.Equal(t, "InProgress", readJob(t, fake, "~1")["status"])
}

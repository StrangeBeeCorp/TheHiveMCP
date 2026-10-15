package execute_automation_test

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/execute_automation"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// Drift the client cannot decode takes other shapes than a wrong date: a page
// that is not JSON, an array where an object is due, entity text in the fields
// an enum would be checked on. Each is still a request TheHive accepted.
//
// spec: VER-002
func TestRunWorkerAcceptedAnswerOfAnyShapeSaysTheHiveAcceptedIt(t *testing.T) {
	t.Parallel()

	const page = `<html><body>` + hostileText + `</body></html>`

	hostileEnums := `{"_id":"~5","_type":"Action","_createdBy":"x","_createdAt":1,"status":"` + hostileText + `","objectType":"` + hostileText + `","startDate":"soon"}`
	hostileJob := `[{"_id":"~6","analyzerName":"` + hostileText + `"}]`

	cases := []struct {
		name       string
		cortex     *testutils.FakeCortex
		params     execute_automation.ExecuteAutomationParams
		wantStatus string
		wantRead   string
	}{
		{"responder answered a page with 202", &testutils.FakeCortex{ActionAnswer: testutils.Answer{Status: 202, Body: page}},
			responderOn(types.EntityTypeCase, recordedCaseID), "202", execute_automation.OperationGetActionStatus},
		{"responder answered entity text in its enums", &testutils.FakeCortex{ActionAnswer: testutils.Answer{Body: hostileEnums}},
			responderOn(types.EntityTypeTask, recordedTaskID), "200", execute_automation.OperationGetActionStatus},
		{"analyzer answered an array with 202", &testutils.FakeCortex{JobAnswer: testutils.Answer{Status: 202, Body: hostileJob}},
			runAnalyzer("VirusTotal_3_0"), "202", execute_automation.OperationGetJobStatus},
		{"analyzer answered a page", &testutils.FakeCortex{JobAnswer: testutils.Answer{Body: page}},
			runAnalyzer("VirusTotal_3_0"), "200", execute_automation.OperationGetJobStatus},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := workerContext(t, noWorkerNamedYAML, tc.cortex)

			_, err := (&execute_automation.ExecuteAutomationTool{}).Handle(ctx, mcp.CallToolRequest{}, tc.params)

			failure := failureOf(t, err)

			assert.Contains(t, strings.ToLower(err.Error()), "accepted")
			assert.Contains(t, err.Error(), tc.wantStatus)
			assert.Contains(t, strings.ToLower(failure.hints()), "again")
			assert.Contains(t, failure.hints(), searchEntitiesTool)
			assert.Contains(t, failure.hints(), tc.wantRead)
			assert.NotContains(t, err.Error(), hostileText, "an error never quotes entity text")
			assert.NotContains(t, err.Error(), "<html>", "an error never quotes the body TheHive sent")
		})
	}
}

// The tool tells the model not to run the responder again, and does not do it
// itself: the one responder TheHive accepted ran once.
func TestRunResponderAcceptedButUnreadableIsNotRetried(t *testing.T) {
	t.Parallel()

	cortex := &testutils.FakeCortex{ActionAnswer: testutils.Answer{Status: 201, Body: `{"_id":"~5","startDate":"soon"}`}}
	ctx := workerContext(t, noWorkerNamedYAML, cortex)

	_, err := (&execute_automation.ExecuteAutomationTool{}).Handle(ctx, mcp.CallToolRequest{}, responderOn(types.EntityTypeCase, recordedCaseID))

	require.Error(t, err)
	assert.Len(t, cortex.Actions(), 1)
}

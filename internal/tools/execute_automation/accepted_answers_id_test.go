package execute_automation_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/execute_automation"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// TheHive created the action or the job and its answer holds the ID, the one
// thing the model needs to read it: get-job-status takes nothing else. The ID is
// a value TheHive generates, so the error may carry it, as it carries a status;
// the rest of the answer is entity text and stays out (UNT-002).
//
// spec: VER-002
func TestRunWorkerAcceptedButUnreadableNamesTheCreatedID(t *testing.T) {
	t.Parallel()

	driftedAction := `{"_id":"` + recordedCaseAction + `","_type":"Action","startDate":"yesterday","report":"` + hostileText + `"}`
	driftedJob := `{"_id":"~91","_type":"Job","startDate":"yesterday","report":{"summary":"` + hostileText + `"}}`

	cases := []struct {
		name   string
		cortex *testutils.FakeCortex
		params execute_automation.ExecuteAutomationParams
		wantID string
	}{
		{"a responder", &testutils.FakeCortex{ActionAnswer: testutils.Answer{Body: driftedAction}}, responderOn(types.EntityTypeCase, recordedCaseID), recordedCaseAction},
		{"an analyzer", &testutils.FakeCortex{JobAnswer: testutils.Answer{Body: driftedJob}}, runAnalyzer("VirusTotal_3_0"), "~91"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := workerContext(t, noWorkerNamedYAML, tc.cortex)

			_, err := (&execute_automation.ExecuteAutomationTool{}).Handle(ctx, mcp.CallToolRequest{}, tc.params)

			failure := failureOf(t, err)
			assert.Contains(t, failure.Message+failure.hints(), tc.wantID, "the error names the ID of what TheHive created")
			assert.NotContains(t, err.Error(), hostileText, "the rest of the answer is entity text")
		})
	}
}

// The ID is read out of an answer the client could not decode, so it is checked
// before it reaches the model: an "_id" that is not an ID is entity text.
//
// spec: VER-002
func TestRunWorkerAcceptedButUnreadableNeverQuotesAnIDThatIsNotOne(t *testing.T) {
	t.Parallel()

	answers := map[string]string{
		"an instruction": `{"_id":"` + hostileText + `","_type":"Action","startDate":"yesterday"}`,
		"a number":       `{"_id":91,"_type":"Action","startDate":"yesterday"}`,
		"an object":      `{"_id":{"x":"` + hostileText + `"},"_type":"Action","startDate":"yesterday"}`,
		"no ID":          `{"_type":"Action","startDate":"yesterday","report":"` + hostileText + `"}`,
	}

	for name, body := range answers {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := workerContext(t, noWorkerNamedYAML, &testutils.FakeCortex{ActionAnswer: testutils.Answer{Body: body}})

			_, err := (&execute_automation.ExecuteAutomationTool{}).Handle(ctx, mcp.CallToolRequest{}, responderOn(types.EntityTypeCase, recordedCaseID))

			failure := failureOf(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), "accepted", "the answer is still reported as accepted")
			assert.Contains(t, strings.ToLower(failure.hints()), "again")
			assert.NotContains(t, err.Error(), hostileText)
			assert.NotContains(t, err.Error(), "IGNORE")
		})
	}
}

// A create that answers 2xx with nothing is not a success (SIL-001): no action
// or job comes back to read, so the model gets the accepted-but-unreadable
// error, with no ID to name, never an empty result.
//
// spec: VER-002
func TestRunWorkerAnsweredWithoutABodyIsAcceptedButUnreadable(t *testing.T) {
	t.Parallel()

	bodies := map[string]string{"empty": "", "whitespace": " \n\t "}

	for _, status := range []int{200, 202, 204} {
		for bodyName, body := range bodies {
			answer := testutils.Answer{Status: status, Body: body}
			workers := map[string]struct {
				cortex *testutils.FakeCortex
				params execute_automation.ExecuteAutomationParams
				read   string
			}{
				workerResponder: {&testutils.FakeCortex{ActionAnswer: answer}, responderOn(types.EntityTypeCase, recordedCaseID), execute_automation.OperationGetActionStatus},
				workerAnalyzer:  {&testutils.FakeCortex{JobAnswer: answer}, runAnalyzer("VirusTotal_3_0"), execute_automation.OperationGetJobStatus},
			}

			for workerName, worker := range workers {
				t.Run(workerName+" "+strconv.Itoa(status)+" "+bodyName, func(t *testing.T) {
					t.Parallel()

					ctx := workerContext(t, noWorkerNamedYAML, worker.cortex)

					result, err := (&execute_automation.ExecuteAutomationTool{}).Handle(ctx, mcp.CallToolRequest{}, worker.params)

					require.Error(t, err, "a create that answers nothing is not a success: %+v", result)

					failure := failureOf(t, err)
					assert.Contains(t, strings.ToLower(err.Error()), "accepted")
					assert.Contains(t, err.Error(), strconv.Itoa(status), "the error carries the HTTP status")
					assert.Contains(t, strings.ToLower(failure.hints()), "again", "the hint tells the model not to run it again")
					assert.Contains(t, failure.hints(), searchEntitiesTool)
					assert.Contains(t, failure.hints(), worker.read)
				})
			}
		}
	}
}

package execute_automation_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/execute_automation"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// A 2xx whose body is not JSON is not TheHive's answer: a reverse proxy or an SSO
// gateway answered a login or a maintenance page, and nothing ran. The error must
// not say the run was created, or the model would never retry it; it says the
// answer was not the expected JSON and sends the model to look first.
//
// spec: VER-002
func TestRunWorkerAnsweredWithAPageThatIsNotJSONDoesNotSayTheRunWasCreated(t *testing.T) {
	t.Parallel()

	page := `<html><body>` + hostileText + `</body></html>`

	for _, status := range []int{200, 202} {
		for _, contentType := range []string{contentTypeHTML, "application/json"} {
			answer := testutils.Answer{Status: status, Body: page, ContentType: contentType}
			workers := map[string]struct {
				cortex *testutils.FakeCortex
				params execute_automation.ExecuteAutomationParams
				entity string
			}{
				workerResponder: {&testutils.FakeCortex{ActionAnswer: answer}, responderOn(types.EntityTypeCase, recordedCaseID), "action"},
				workerAnalyzer:  {&testutils.FakeCortex{JobAnswer: answer}, runAnalyzer("VirusTotal_3_0"), "job"},
			}

			for workerName, worker := range workers {
				t.Run(workerName+" "+strconv.Itoa(status)+" "+contentType, func(t *testing.T) {
					t.Parallel()

					ctx := workerContext(t, noWorkerNamedYAML, worker.cortex)

					_, err := (&execute_automation.ExecuteAutomationTool{}).Handle(ctx, mcp.CallToolRequest{}, worker.params)

					failure := failureOf(t, err)
					text := strings.ToLower(err.Error())

					assert.Contains(t, text, "not the expected json", "the error says what was wrong with the answer")
					assert.Contains(t, err.Error(), strconv.Itoa(status), "the error carries the HTTP status")
					assert.Contains(t, text, "may not have", "the run may not have happened")
					assert.Contains(t, failure.hints(), searchEntitiesTool, "the hint sends the model to look first")
					assert.Contains(t, failure.hints(), "entity-type "+worker.entity)
					assert.NotContains(t, text, "created", "nothing is known to have been created")
					assert.NotContains(t, text, "do not run it again")
					assert.NotContains(t, err.Error(), hostileText, "an error never quotes entity text")
					assert.NotContains(t, err.Error(), "<html>", "an error never quotes the body TheHive sent")
				})
			}
		}
	}
}

// A JSON answer the client cannot decode is bigger than what is read of it to find
// its ID when a Cortex report is large. Cutting it must not make it look like a
// page: it is still TheHive's JSON, and the run was created.
//
// spec: VER-002
func TestRunWorkerLargeJSONAnswerTheClientCannotDecodeStaysAccepted(t *testing.T) {
	t.Parallel()

	large := `{"_id":"~5","_type":"Job","startDate":"soon","report":"` + strings.Repeat("a", 2<<20) + `"}`

	for workerName, worker := range map[string]struct {
		cortex *testutils.FakeCortex
		params execute_automation.ExecuteAutomationParams
	}{
		workerResponder: {&testutils.FakeCortex{ActionAnswer: testutils.Answer{Body: large}}, responderOn(types.EntityTypeCase, recordedCaseID)},
		workerAnalyzer:  {&testutils.FakeCortex{JobAnswer: testutils.Answer{Body: large}}, runAnalyzer("VirusTotal_3_0")},
	} {
		t.Run(workerName, func(t *testing.T) {
			t.Parallel()

			ctx := workerContext(t, noWorkerNamedYAML, worker.cortex)

			_, err := (&execute_automation.ExecuteAutomationTool{}).Handle(ctx, mcp.CallToolRequest{}, worker.params)

			failure := failureOf(t, err)

			assert.Contains(t, strings.ToLower(err.Error()), "accepted")
			assert.Contains(t, strings.ToLower(failure.hints()), "do not run it again")
			assert.NotContains(t, strings.ToLower(err.Error()), "not the expected json", "a large JSON answer is not a page")
		})
	}
}

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

type sealedWorker struct {
	cortex *testutils.FakeCortex
	params execute_automation.ExecuteAutomationParams
	entity string
}

func sealedWorkers(answer testutils.Answer) map[string]sealedWorker {
	return map[string]sealedWorker{
		workerResponder: {&testutils.FakeCortex{ActionAnswer: answer}, responderOn(types.EntityTypeCase, recordedCaseID), "action"},
		workerAnalyzer:  {&testutils.FakeCortex{JobAnswer: answer}, runAnalyzer("VirusTotal_3_0"), "job"},
	}
}

// What is not a JSON object or array is not TheHive's answer, whatever it looks
// like: plain text, a bare scalar, a cut-off document, a page that quotes JSON.
//
// spec: VER-002
func TestSealedRunWorkerBodyThatIsNotAJSONDocumentIsNotReportedAsCreated(t *testing.T) {
	t.Parallel()

	bodies := map[string]testutils.Answer{
		"plain text":              {Body: "OK"},
		"a bare string":           {Body: `"ok"`},
		"a bare number":           {Body: `42`},
		"a cut-off object":        {Body: `{"_id":"~5","startDate":`},
		"a page after whitespace": {Body: "\n\n  <!DOCTYPE html><html>" + hostileText + "</html>", ContentType: "text/html; charset=utf-8"},
		"a page quoting an ID":    {Status: 202, Body: `<html>{"_id":"~5"} ` + hostileText + `</html>`, ContentType: contentTypeHTML},
	}

	for bodyName, answer := range bodies {
		for workerName, worker := range sealedWorkers(answer) {
			t.Run(workerName+" "+bodyName, func(t *testing.T) {
				t.Parallel()

				ctx := workerContext(t, noWorkerNamedYAML, worker.cortex)

				_, err := (&execute_automation.ExecuteAutomationTool{}).Handle(ctx, mcp.CallToolRequest{}, worker.params)

				failure := failureOf(t, err)
				text := strings.ToLower(err.Error())
				status := answer.Status

				if status == 0 {
					status = 200
				}

				assert.Contains(t, text, "not the expected json")
				assert.Contains(t, err.Error(), strconv.Itoa(status))
				assert.Contains(t, text, "may not have")
				assert.Contains(t, failure.hints(), searchEntitiesTool)
				assert.Contains(t, failure.hints(), "entity-type "+worker.entity)
				assert.NotContains(t, text, "created")
				assert.NotContains(t, text, "do not run it again")
				assert.NotContains(t, err.Error(), "~5", "an ID found inside a page is not an ID TheHive gave")
				assert.NotContains(t, err.Error(), hostileText)
			})
		}
	}
}

// A body that IS a JSON document keeps the accepted error, however it is
// padded or whatever its strings look like, and whatever the Content-Type says.
//
// spec: VER-002
func TestSealedRunWorkerJSONBodyTheClientCannotDecodeStaysAccepted(t *testing.T) {
	t.Parallel()

	const drifted = `{"_id":"~5","_type":"Action","startDate":"soon"`

	bodies := map[string]testutils.Answer{
		"padded with whitespace":        {Body: "\n  " + drifted + `}` + " \n"},
		"a string that looks like HTML": {Body: drifted + `,"report":"<html>` + hostileText + `</html>"}`},
		"served as text/html":           {Body: drifted + `}`, ContentType: contentTypeHTML},
	}

	for bodyName, answer := range bodies {
		for workerName, worker := range sealedWorkers(answer) {
			t.Run(workerName+" "+bodyName, func(t *testing.T) {
				t.Parallel()

				ctx := workerContext(t, noWorkerNamedYAML, worker.cortex)

				_, err := (&execute_automation.ExecuteAutomationTool{}).Handle(ctx, mcp.CallToolRequest{}, worker.params)

				failure := failureOf(t, err)

				assert.Contains(t, strings.ToLower(err.Error()), "accepted")
				assert.Contains(t, strings.ToLower(failure.hints()), "do not run it again")
				assert.NotContains(t, strings.ToLower(err.Error()), "not the expected json")
				assert.NotContains(t, err.Error(), hostileText)
			})
		}
	}
}

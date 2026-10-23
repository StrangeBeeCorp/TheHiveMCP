package utils

import (
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/assert"
)

// The 6.0 client lists analyzers and responders in per-operation types; the tools read them as
// OutputWorker, and nothing of a worker may be lost on the way: its ID is what a run takes, and
// its name and Cortex what the permissions and cortex-id use.
func TestOutputWorkersKeepEveryFieldOfEachListingShape(t *testing.T) {
	t.Parallel()

	want := thehive.OutputWorker{
		Id: "5a142b9f", Name: "TheHiveMCP_Test_Analyzer_1_0", Version: "1.0", Description: "test neuron",
		DataTypeList: []string{"ip", "fqdn"}, CortexIds: []string{"local", "cortex0"},
	}

	assert.Equal(t, []thehive.OutputWorker{want}, OutputWorkers([]thehive.AnalyzerOutput{thehive.AnalyzerOutput(want)}))
	assert.Equal(t, []thehive.OutputWorker{want}, OutputWorkers([]thehive.ListRespondersOutput{thehive.ListRespondersOutput(want)}))
	assert.Equal(t, []thehive.OutputWorker{want}, OutputWorkers([]thehive.OutputWorker{want}))
}

func TestOutputWorkersOfNoWorkerIsAnEmptyList(t *testing.T) {
	t.Parallel()

	assert.Empty(t, OutputWorkers([]thehive.AnalyzerOutput(nil)))
	assert.NotNil(t, OutputWorkers([]thehive.ListRespondersOutput(nil)), "an empty list, which marshals as [], not null")
}

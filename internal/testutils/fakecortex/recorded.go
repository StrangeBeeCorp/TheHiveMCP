package fakecortex

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// recordedJobFile is the job TheHive 6.0 answered once Cortex had finished it.
// It sits with the other recorded answers of testutils, which this package
// cannot import (testutils imports the tools), so it is read from there.
const recordedJobFile = "job-finished.json"

func loadRecordedJob() ([]byte, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return nil, errors.New("cannot locate the recorded job: no caller source path")
	}

	path := filepath.Join(filepath.Dir(thisFile), "..", "testdata", "recorded", recordedJobFile)

	raw, err := os.ReadFile(path) // #nosec G304 -- a fixed recorded file next to this package's source
	if err != nil {
		return nil, fmt.Errorf("read the recorded job: %w", err)
	}

	return raw, nil
}

// recordedJob answers the recorded job for jobID in that Cortex state. Only
// what the fake plays is overridden: the identifier, the status and the report
// a test reads, which gets the summary the recording lacks. A job that has not
// finished has no end date.
func recordedJob(jobID, status string) ([]byte, error) {
	raw, err := loadRecordedJob()
	if err != nil {
		return nil, err
	}

	var job map[string]any

	err = json.Unmarshal(raw, &job)
	if err != nil {
		return nil, fmt.Errorf("decode the recorded job: %w", err)
	}

	job["_id"] = jobID
	job["id"] = jobID
	job["status"] = status
	job["cortexJobId"] = "cortex-" + jobID

	if status != "Success" {
		delete(job, "endDate")
	}

	report, _ := job["report"].(map[string]any)
	if report == nil {
		report = map[string]any{}
	}

	report["summary"] = map[string]any{"taxonomies": []any{map[string]any{"level": "info", "namespace": "crtsh", "value": "0 certificates"}}}
	job["report"] = report

	body, err := json.Marshal(job)
	if err != nil {
		return nil, fmt.Errorf("encode the job: %w", err)
	}

	return body, nil
}

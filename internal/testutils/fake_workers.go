package testutils

import (
	"encoding/json"
	"fmt"
	"maps"
	"testing"
)

// Worker is one analyzer or responder as TheHive sent it: a JSON object served
// as it is, the fields the client does not model (cortexIds) included. Take one
// from the recorded catalogs and override only the fields a test cares about.
type Worker map[string]any

const (
	analyzerCatalog  = "analyzer-catalog.json"
	responderCatalog = "responder-catalog-case.json"
)

// RecordedAnalyzer returns the analyzer of that name from TheHive's recorded
// analyzer catalog, and fails the test when the catalog has none.
func RecordedAnalyzer(t *testing.T, name string) Worker {
	t.Helper()

	return recordedWorker(t, analyzerCatalog, name)
}

// RecordedResponder returns the responder of that name from TheHive's recorded
// responder catalog, and fails the test when the catalog has none.
func RecordedResponder(t *testing.T, name string) Worker {
	t.Helper()

	return recordedWorker(t, responderCatalog, name)
}

func recordedWorker(t *testing.T, file, name string) Worker {
	t.Helper()

	var catalog []Worker

	err := json.Unmarshal([]byte(RecordedAnswer(t, file)), &catalog)
	if err != nil {
		t.Fatalf("recorded catalog %s is not a list of workers: %v", file, err)
	}

	for _, worker := range catalog {
		if worker.Name() == name {
			return worker
		}
	}

	t.Fatalf("recorded catalog %s has no worker %q", file, name)

	return nil
}

// With returns a copy of the worker with one field overridden; the receiver and
// the recorded entry it came from are left as they were.
func (w Worker) With(field string, value any) Worker {
	copied := maps.Clone(w)
	copied[field] = value

	return copied
}

// ID is the worker's id.
func (w Worker) ID() string {
	id, _ := w["id"].(string)

	return id
}

// Name is the worker's name.
func (w Worker) Name() string {
	name, _ := w["name"].(string)

	return name
}

// catalogBody marshals workers as a JSON list: [] and never null when there are
// none, as TheHive answers.
func catalogBody(workers []Worker) ([]byte, error) {
	if workers == nil {
		workers = []Worker{}
	}

	body, err := json.Marshal(workers)
	if err != nil {
		return nil, fmt.Errorf("encode the catalog: %w", err)
	}

	return body, nil
}

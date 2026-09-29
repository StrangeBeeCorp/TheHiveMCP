// Package fakecortex answers TheHive's Cortex job endpoint from memory, for unit
// tests of code that polls analyzer jobs. It is a package of its own so a tool
// package can use it from its internal tests: testutils imports the tools, and
// importing net/http directly makes bodyclose flag every SDK call in the
// package under test.
package fakecortex

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

const (
	jobPath   = "/api/v1/connector/cortex/job/"
	queryPath = "/api/v1/query"
)

// Reply is one answer of the fake for a job.
type Reply struct {
	status string
	code   int
	hang   bool
}

// Status answers with the job in that Cortex state, report included.
func Status(status string) Reply { return Reply{status: status} }

// Unavailable answers 502, a failure worth asking again.
func Unavailable() Reply { return Reply{code: http.StatusBadGateway} }

// Hang holds the request until the caller abandons it.
func Hang() Reply { return Reply{hang: true} }

// Jobs answers each job's replies in turn, repeating the last one. A job it
// has no reply for is answered 404. It also answers the scope query TheHiveMCP
// runs before reading a job: every job is in scope unless OutOfScope says not.
type Jobs struct {
	mu         sync.Mutex
	replies    map[string][]Reply
	outOfScope map[string]bool
	reads      map[string]int
}

// NewJobs returns a fake answering replies, keyed by job ID.
func NewJobs(replies map[string][]Reply) *Jobs {
	return &Jobs{replies: replies, outOfScope: map[string]bool{}, reads: map[string]int{}}
}

// OutOfScope makes the scope query exclude these jobs.
func (j *Jobs) OutOfScope(jobIDs ...string) *Jobs {
	for _, jobID := range jobIDs {
		j.outOfScope[jobID] = true
	}

	return j
}

// Reads reports how many times the job itself was read.
func (j *Jobs) Reads(jobID string) int {
	j.mu.Lock()
	defer j.mu.Unlock()

	return j.reads[jobID]
}

// Client starts the fake and returns a TheHive client pointed at it.
func (j *Jobs) Client(t *testing.T) *thehive.APIClient {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(j.serve))
	t.Cleanup(server.Close)

	cfg := thehive.NewConfiguration()
	cfg.Servers = thehive.ServerConfigurations{{URL: server.URL}}

	return thehive.NewAPIClient(cfg)
}

func (j *Jobs) next(jobID string) (Reply, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.reads[jobID]++

	replies := j.replies[jobID]
	if len(replies) == 0 {
		return Reply{}, false
	}

	if len(replies) > 1 {
		j.replies[jobID] = replies[1:]
	}

	return replies[0], true
}

func (j *Jobs) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == queryPath {
		j.serveScope(w, r)

		return
	}

	jobID, isJob := strings.CutPrefix(r.URL.Path, jobPath)
	reply, known := j.next(jobID)

	switch {
	case !isJob || !known:
		w.WriteHeader(http.StatusNotFound)
	case reply.hang:
		<-r.Context().Done()
	case reply.code != 0:
		w.WriteHeader(reply.code)
	default:
		body, err := json.Marshal(thehive.OutputJob{
			UnderscoreId: jobID, UnderscoreType: "Job", UnderscoreCreatedBy: "test", UnderscoreCreatedAt: 1, Id: jobID,
			AnalyzerId: "a0b1c2", AnalyzerName: "VirusTotal_3_0", AnalyzerDefinition: "VirusTotal_3_0",
			Status: reply.status, StartDate: 1, CortexId: "local", CortexJobId: "cortex-" + jobID,
			Report: map[string]any{
				"success":   true,
				"summary":   map[string]any{"taxonomies": []any{map[string]any{"level": "malicious", "namespace": "VT", "value": "42/70"}}},
				"full":      map[string]any{"raw": strings.Repeat("x", 4096)},
				"artifacts": []any{map[string]any{"dataType": "domain", "data": "evil.example"}},
			},
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}
}

// serveScope answers the getJob → observable → filter query with one row for
// a job in scope and none for a job out of it.
func (j *Jobs) serveScope(w http.ResponseWriter, r *http.Request) {
	var query struct {
		Query []map[string]any `json:"query"`
	}

	err := json.NewDecoder(r.Body).Decode(&query)
	if err != nil || len(query.Query) == 0 {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	jobID, _ := query.Query[0]["idOrName"].(string)
	rows := []any{map[string]any{"_id": "~observable-of-" + jobID}}

	j.mu.Lock()
	if j.outOfScope[jobID] {
		rows = []any{}
	}
	j.mu.Unlock()

	body, err := json.Marshal(rows)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

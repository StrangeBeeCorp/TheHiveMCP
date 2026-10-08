package dashboard

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeJira answers Jira's search a page at a time, and records the requests.
type fakeJira struct {
	pages    []string
	status   int
	requests []map[string]any
	auth     []string
}

func (f *fakeJira) Do(req *http.Request) (*http.Response, error) {
	var body map[string]any

	_ = json.NewDecoder(req.Body).Decode(&body)
	f.requests = append(f.requests, body)

	user, _, _ := req.BasicAuth()
	f.auth = append(f.auth, user)

	status := f.status
	if status == 0 {
		status = http.StatusOK
	}

	page := f.pages[min(len(f.requests), len(f.pages))-1]

	return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(page))}, nil
}

func jiraEnv(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

var jiraSet = map[string]string{
	EnvJiraURL: "https://site.atlassian.net/", EnvJiraEmail: "me@example.com", EnvJiraToken: "t", EnvJiraJQL: "project = X",
}

// Jira's done tickets are read page by page, each with its first move to In Progress; a ticket
// never started is left out.
func TestJiraGivesEachTicketsLeadTime(t *testing.T) {
	t.Parallel()

	f := &fakeJira{pages: []string{
		`{"issues":[{"key":"X-1","fields":{"summary":"one","resolutiondate":"2026-10-05T10:00:00.000+0000"},"changelog":{"histories":[` +
			`{"created":"2026-10-03T10:00:00.000+0000","items":[{"field":"status","toString":"In Progress"}]},` +
			`{"created":"2026-10-01T10:00:00.000+0000","items":[{"field":"status","toString":"In Progress"}]}]}}],"nextPageToken":"p2","isLast":false}`,
		`{"issues":[{"key":"X-2","fields":{"summary":"never started","resolutiondate":"2026-10-05T10:00:00.000+0000"},"changelog":{"histories":[]}}],"isLast":true}`,
	}}

	tickets, err := readJira(t.Context(), Options{Getenv: jiraEnv(jiraSet), HTTP: f})
	require.NoError(t, err)
	require.Len(t, tickets, 1)
	assert.Equal(t, "X-1", tickets[0].Key)
	assert.Equal(t, "2026-10-01", tickets[0].Started.Format("2006-01-02"), "the first move to In Progress")

	require.Len(t, f.requests, 2)
	assert.NotContains(t, f.requests[0], "nextPageToken")
	assert.Equal(t, "p2", f.requests[1]["nextPageToken"])
	assert.Equal(t, "me@example.com", f.auth[0])
}

// Jira unconfigured names what to set; a refused search says Jira's answer.
func TestJiraUnconfiguredOrRefusingSaysWhy(t *testing.T) {
	t.Parallel()

	_, err := readJira(t.Context(), Options{Getenv: jiraEnv(map[string]string{EnvJiraURL: "x"}), HTTP: &fakeJira{}})
	require.EqualError(t, err, "not configured: set JIRA_EMAIL, JIRA_API_TOKEN, JIRA_JQL")

	_, err = readJira(t.Context(), Options{})
	require.Error(t, err)

	_, err = readJira(t.Context(), Options{Getenv: jiraEnv(jiraSet), HTTP: &fakeJira{pages: []string{"{}"}, status: http.StatusUnauthorized}})
	require.ErrorIs(t, err, errStatus)
	assert.Contains(t, err.Error(), "Unauthorized")
}

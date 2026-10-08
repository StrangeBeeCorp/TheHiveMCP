package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// The environment the optional sources read their credentials from.
const (
	EnvJiraURL   = "JIRA_BASE_URL"  // https://<site>.atlassian.net
	EnvJiraEmail = "JIRA_EMAIL"     // the token's account
	EnvJiraToken = "JIRA_API_TOKEN" // #nosec G101 -- the variable that holds an Atlassian API token, not one
	EnvJiraJQL   = "JIRA_JQL"       // the tickets whose lead time the page charts
	jiraPage     = 100
	maxBody      = 32 << 20
)

// HTTPDoer sends a request to Jira: an *http.Client, or a test's fake.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// unsetError says which variables a source needs.
type unsetError []string

func (e unsetError) Error() string {
	return "not configured: set " + strings.Join(e, ", ")
}

// Ticket is a ticket done, and when it was started.
type Ticket struct {
	Key     string
	Summary string
	Started time.Time
	Done    time.Time
}

func unset(getenv func(string) string, keys ...string) unsetError {
	var missing unsetError

	for _, k := range keys {
		if getenv(k) == "" {
			missing = append(missing, k)
		}
	}

	return missing
}

// readJira reads the done tickets JIRA_JQL selects, with the date each first went In Progress.
func readJira(ctx context.Context, o Options) ([]Ticket, error) {
	if o.HTTP == nil || o.Getenv == nil {
		return nil, unsetError{EnvJiraURL, EnvJiraEmail, EnvJiraToken, EnvJiraJQL}
	}

	missing := unset(o.Getenv, EnvJiraURL, EnvJiraEmail, EnvJiraToken, EnvJiraJQL)
	if len(missing) > 0 {
		return nil, missing
	}

	var (
		tickets []Ticket
		token   string
	)

	for {
		page, err := searchJira(ctx, o, token)
		if err != nil {
			return nil, err
		}

		for _, i := range page.Issues {
			if t, ok := i.ticket(); ok {
				tickets = append(tickets, t)
			}
		}

		if page.IsLast || page.NextPageToken == "" {
			return tickets, nil
		}

		token = page.NextPageToken
	}
}

// searchJira asks for one page of the search, the first when token is "".
func searchJira(ctx context.Context, o Options, token string) (jiraSearch, error) {
	query := map[string]any{
		"jql": o.Getenv(EnvJiraJQL), "fields": []string{"summary", "resolutiondate"},
		"expand": "changelog", "maxResults": jiraPage,
	}
	if token != "" {
		query["nextPageToken"] = token
	}

	var page jiraSearch

	body, err := json.Marshal(query)
	if err != nil {
		return page, fmt.Errorf("jira: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(o.Getenv(EnvJiraURL), "/")+"/rest/api/3/search/jql", bytes.NewReader(body))
	if err != nil {
		return page, fmt.Errorf("jira: %w", err)
	}

	req.SetBasicAuth(o.Getenv(EnvJiraEmail), o.Getenv(EnvJiraToken))
	req.Header.Set("Content-Type", "application/json")

	err = fetchJSON(o.HTTP, req, &page)
	if err != nil {
		return page, fmt.Errorf("jira: %w", err)
	}

	return page, nil
}

type jiraSearch struct {
	Issues        []jiraIssue `json:"issues"`
	NextPageToken string      `json:"nextPageToken"`
	IsLast        bool        `json:"isLast"`
}

type jiraIssue struct {
	Key    string `json:"key"`
	Fields struct {
		Summary    string `json:"summary"`
		Resolution string `json:"resolutiondate"`
	} `json:"fields"`
	Changelog struct {
		Histories []struct {
			Created string `json:"created"`
			Items   []struct {
				Field    string `json:"field"`
				ToString string `json:"toString"`
			} `json:"items"`
		} `json:"histories"`
	} `json:"changelog"`
}

// jiraTime is the layout of Jira's dates, 2026-10-08T11:17:35.000+0200.
const jiraTime = "2006-01-02T15:04:05.000-0700"

// ticket is the issue's first move to In Progress and its resolution; false when it has neither.
func (i jiraIssue) ticket() (Ticket, bool) {
	done, err := time.Parse(jiraTime, i.Fields.Resolution)
	if err != nil {
		return Ticket{}, false
	}

	t := Ticket{Key: i.Key, Summary: i.Fields.Summary, Done: done}

	for _, h := range i.Changelog.Histories {
		when, err := time.Parse(jiraTime, h.Created)
		if err != nil {
			continue
		}

		for _, it := range h.Items {
			if it.Field == "status" && strings.EqualFold(it.ToString, "In Progress") && (t.Started.IsZero() || when.Before(t.Started)) {
				t.Started = when
			}
		}
	}

	return t, !t.Started.IsZero() && !t.Started.After(t.Done)
}

// errStatus is an answer other than 200.
var errStatus = errors.New("unexpected answer")

func fetchJSON(c HTTPDoer, req *http.Request, into any) error {
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", req.URL.Host, err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return fmt.Errorf("%s: %w", req.URL.Host, err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w from %s: %s", errStatus, req.URL.Host, resp.Status)
	}

	err = json.Unmarshal(data, into)
	if err != nil {
		return fmt.Errorf("%s: %w", req.URL.Host, err)
	}

	return nil
}

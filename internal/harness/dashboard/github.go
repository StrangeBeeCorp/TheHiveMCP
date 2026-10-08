package dashboard

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/issues"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/triage"
)

// GitHub runs gh in the checkout and returns what it prints.
type GitHub interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// The GitHub sources, each named on the page when it cannot be read.
const (
	SourcePRs     = SourceGitHub + ": pull requests"
	SourceCI      = SourceGitHub + ": CI runs"
	SourceNightly = SourceGitHub + ": nightly runs"
	SourceIssues  = SourceGitHub + ": issues"
)

// The workflows the page reads the runs of.
const (
	ciWorkflow      = "ci.yml"
	nightlyWorkflow = "nightly.yml"
	runLimit        = "300"
	issueLimit      = "500"
)

// errNoGH means there is no gh to ask.
var errNoGH = errors.New("gh is not installed or not logged in (gh auth login)")

// GitHubData is what GitHub says of the repository.
type GitHubData struct {
	PRs      []PR
	CI       []Run
	Nightly  []Run
	Findings []Issue // open nightly findings
	Debt     []Issue // open debt issues
}

// PR is a merged pull request, and what its description says of its review.
type PR struct {
	Number      int
	Title       string
	Base        string
	Created     time.Time
	Merged      time.Time
	FirstCommit time.Time
	// Grade is the description's /10 grade, 0 for none.
	Grade int
	// ReviewTokens and Fixed are the review's spend and the findings it fixed, 0 when unsaid.
	ReviewTokens int
	Fixed        int
}

// Run is one workflow run.
type Run struct {
	Created    time.Time `json:"createdAt"`
	Updated    time.Time `json:"updatedAt"`
	Conclusion string    `json:"conclusion"`
	Attempt    int       `json:"attempt"`
	Branch     string    `json:"headBranch"`
	Event      string    `json:"event"`
}

// Issue is an open issue.
type Issue struct {
	Number  int       `json:"number"`
	Title   string    `json:"title"`
	Created time.Time `json:"createdAt"`
}

var (
	gradeBadge  = regexp.MustCompile(`review-(\d+)%2F10`)
	tokensSpent = regexp.MustCompile(`([\d,]+) of [\d,]+ tokens`)
	fixedCount  = regexp.MustCompile(`· (\d+) fixed`)
)

// prQuery asks for the merged pull requests with their first commit, a page at a time (gh's
// --json commits asks for every commit's authors, which GitHub refuses past a few hundred).
const prQuery = `query($owner: String!, $name: String!, $endCursor: String) {
  repository(owner: $owner, name: $name) {
    pullRequests(states: MERGED, first: 100, after: $endCursor, orderBy: {field: CREATED_AT, direction: DESC}) {
      pageInfo { hasNextPage endCursor }
      nodes { number title body baseRefName createdAt mergedAt commits(first: 1) { nodes { commit { authoredDate } } } }
    }
  }
}`

// readGitHub asks gh for each part at once; a part that fails is named, the others are kept.
func readGitHub(ctx context.Context, gh GitHub) (GitHubData, map[string]error) {
	if gh == nil {
		return GitHubData{}, map[string]error{SourceGitHub: errNoGH}
	}

	var (
		d    GitHubData
		mu   sync.Mutex
		errs = map[string]error{}
		wg   sync.WaitGroup
	)

	part := func(source string, read func() error) {
		wg.Go(func() {
			err := read()
			if err != nil {
				mu.Lock()
				errs[source] = err
				mu.Unlock()
			}
		})
	}

	part(SourcePRs, func() (err error) {
		d.PRs, err = readPRs(ctx, gh)
		return err
	})
	part(SourceCI, func() (err error) {
		d.CI, err = readRuns(ctx, gh, ciWorkflow)
		return err
	})
	part(SourceNightly, func() (err error) {
		d.Nightly, err = readRuns(ctx, gh, nightlyWorkflow)
		return err
	})
	part(SourceIssues, func() error {
		var err error

		d.Findings, err = readIssues(ctx, gh, triage.Label)
		if err != nil {
			return err
		}

		d.Debt, err = readIssues(ctx, gh, issues.DebtLabel)

		return err
	})

	wg.Wait()

	return d, errs
}

func readPRs(ctx context.Context, gh GitHub) ([]PR, error) {
	out, err := gh.Run(ctx, "api", "graphql", "--paginate", "-F", "owner={owner}", "-F", "name={repo}", "-f", "query="+prQuery,
		"--jq", ".data.repository.pullRequests.nodes[]")
	if err != nil {
		return nil, fmt.Errorf("the pull requests: %w", err)
	}

	var prs []PR

	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 1<<20), 1<<24) //nolint:mnd // a description may be long: up to 16 MiB

	for sc.Scan() {
		var n struct {
			Number  int       `json:"number"`
			Title   string    `json:"title"`
			Body    string    `json:"body"`
			Base    string    `json:"baseRefName"`
			Created time.Time `json:"createdAt"`
			Merged  time.Time `json:"mergedAt"`
			Commits struct {
				Nodes []struct {
					Commit struct {
						Authored time.Time `json:"authoredDate"`
					} `json:"commit"`
				} `json:"nodes"`
			} `json:"commits"`
		}

		err := json.Unmarshal(sc.Bytes(), &n)
		if err != nil {
			return nil, fmt.Errorf("read gh's pull requests: %w", err)
		}

		pr := PR{Number: n.Number, Title: n.Title, Base: n.Base, Created: n.Created, Merged: n.Merged, FirstCommit: n.Created}
		if len(n.Commits.Nodes) > 0 && !n.Commits.Nodes[0].Commit.Authored.IsZero() {
			pr.FirstCommit = n.Commits.Nodes[0].Commit.Authored
		}

		pr.Grade, pr.ReviewTokens, pr.Fixed = reviewOf(n.Body)
		prs = append(prs, pr)
	}

	err = sc.Err()
	if err != nil {
		return nil, fmt.Errorf("read gh's pull requests: %w", err)
	}

	return prs, nil
}

// reviewOf reads a description's grade badge, review tokens and fixed findings; 0 for what it
// does not say.
func reviewOf(body string) (grade, tokens, fixed int) {
	if m := gradeBadge.FindStringSubmatch(body); m != nil {
		grade, _ = strconv.Atoi(m[1])
	}

	if m := tokensSpent.FindStringSubmatch(body); m != nil {
		tokens, _ = strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
	}

	if m := fixedCount.FindStringSubmatch(body); m != nil {
		fixed, _ = strconv.Atoi(m[1])
	}

	return grade, tokens, fixed
}

func readRuns(ctx context.Context, gh GitHub, workflow string) ([]Run, error) {
	out, err := gh.Run(ctx, "run", "list", "--workflow", workflow, "--limit", runLimit,
		"--json", "createdAt,updatedAt,conclusion,attempt,headBranch,event")
	if err != nil {
		return nil, fmt.Errorf("the runs of %s: %w", workflow, err)
	}

	var runs []Run

	err = json.Unmarshal(out, &runs)
	if err != nil {
		return nil, fmt.Errorf("read gh's runs of %s: %w", workflow, err)
	}

	return runs, nil
}

func readIssues(ctx context.Context, gh GitHub, label string) ([]Issue, error) {
	out, err := gh.Run(ctx, "issue", "list", "--label", label, "--state", "open", "--limit", issueLimit, "--json", "number,title,createdAt")
	if err != nil {
		return nil, fmt.Errorf("the %s issues: %w", label, err)
	}

	var found []Issue

	err = json.Unmarshal(out, &found)
	if err != nil {
		return nil, fmt.Errorf("read gh's %s issues: %w", label, err)
	}

	return found, nil
}

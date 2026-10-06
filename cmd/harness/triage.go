package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/triage"
)

// blockedPath is where an agent that stayed red for its rounds writes why (internal/harness/hooks).
const blockedPath = ".cache/harness/BLOCKED.md"

// errNoGH means gh is not there to read or write the findings.
var errNoGH = errors.New("gh is needed for the nightly findings (install it and run gh auth login)")

// triageNight files the night's failures as GitHub issues labelled nightly, comments on those
// still failing and closes those that pass again (the nightly workflow's last job).
func (e env) triageNight(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("triage", flag.ContinueOnError)
	runs := fs.String("runs", "", "the directory of the night's artifacts, one subdirectory per shard")
	runURL := fs.String("run-url", "", "the nightly run's URL, quoted in each issue")
	expect := fs.String("expect", "", "the shards the night has, comma-separated: one with no result is a finding")
	dry := fs.Bool("dry-run", false, "print what would change, change nothing")

	err := fs.Parse(args)
	if err != nil {
		return usageError{err.Error()}
	}

	if *runs == "" || *runURL == "" {
		return usageError{"triage: --runs and --run-url are needed"}
	}

	var shards []string
	if *expect != "" {
		shards = strings.Split(*expect, ",")
	}

	outcomes, err := triage.Read(*runs, shards)
	if err != nil {
		return fmt.Errorf("triage: %w", err)
	}

	gh := ghIssues{ctx: ctx, dir: e.root}

	open, err := gh.list()
	if err != nil {
		return err
	}

	actions := triage.Plan(outcomes, open, *runURL)
	for _, a := range actions {
		_, _ = fmt.Fprintln(out, describe(a))
	}

	if *dry {
		return nil
	}

	if slices.ContainsFunc(actions, func(a triage.Action) bool { return a.Kind == triage.Open }) {
		err = gh.label()
		if err != nil {
			return err
		}
	}

	for _, a := range actions {
		err = gh.apply(a)
		if err != nil {
			return err
		}
	}

	_, _ = fmt.Fprintf(out, "%d finding(s) changed, %d open before this night\n", len(actions), len(open))

	return nil
}

func describe(a triage.Action) string {
	if a.Kind == triage.Open {
		return "open: " + a.Title
	}

	return fmt.Sprintf("%s #%d", a.Kind, a.Issue)
}

// next lists the open findings, the oldest first, after an agent's BLOCKED.md if there is one
// (make next: what to work on).
func (e env) next(ctx context.Context, out io.Writer) error {
	blocked, err := os.ReadFile(filepath.Join(e.root, blockedPath)) // #nosec G304 -- this checkout's own cache
	if err == nil {
		_, _ = fmt.Fprintf(out, "An agent is blocked (%s):\n\n%s\n", blockedPath, strings.TrimSpace(string(blocked)))
	}

	issues, err := ghIssues{ctx: ctx, dir: e.root}.listForNext()
	if err != nil {
		return err
	}

	if len(issues) == 0 {
		_, _ = fmt.Fprintln(out, "No open nightly finding.")

		return nil
	}

	_, _ = fmt.Fprintf(out, "%d open nightly finding(s), the oldest first:\n", len(issues))

	for _, i := range issues {
		_, _ = fmt.Fprintf(out, "  #%d  %s  (since %s)  %s\n", i.Number, i.Title, i.CreatedAt.Format(time.DateOnly), i.URL)
	}

	return nil
}

// ghIssues reads and writes the nightly findings with gh, in the repository of dir.
type ghIssues struct {
	ctx context.Context //nolint:containedctx // one triage's calls share its context
	dir string
}

// listed is an issue as `gh issue list --json` prints it.
type listed struct {
	triage.Issue

	URL       string    `json:"url"`
	CreatedAt time.Time `json:"createdAt"`
}

func (g ghIssues) list() ([]triage.Issue, error) {
	found, err := g.query("number,title,body")
	if err != nil {
		return nil, err
	}

	issues := make([]triage.Issue, 0, len(found))
	for _, l := range found {
		issues = append(issues, l.Issue)
	}

	return issues, nil
}

func (g ghIssues) listForNext() ([]listed, error) {
	found, err := g.query("number,title,url,createdAt")
	if err != nil {
		return nil, err
	}

	slices.SortFunc(found, func(a, b listed) int { return a.CreatedAt.Compare(b.CreatedAt) })

	return found, nil
}

func (g ghIssues) query(fields string) ([]listed, error) {
	data, err := g.run(nil, "issue", "list", "--label", triage.Label, "--state", "open", "--limit", "500", "--json", fields)
	if err != nil {
		return nil, err
	}

	var found []listed

	err = json.Unmarshal(data, &found)
	if err != nil {
		return nil, fmt.Errorf("read gh's issue list: %w", err)
	}

	return found, nil
}

// label makes sure the nightly label exists before an issue carries it.
func (g ghIssues) label() error {
	_, err := g.run(nil, "label", "create", triage.Label, "--force", "--color", "B60205", "--description", "A finding of the nightly run, filed and closed by bin/harness triage")

	return err
}

func (g ghIssues) apply(a triage.Action) error {
	var err error

	switch a.Kind {
	case triage.Open:
		_, err = g.run(strings.NewReader(a.Body), "issue", "create", "--title", a.Title, "--label", triage.Label, "--body-file", "-")
	case triage.Comment:
		_, err = g.run(strings.NewReader(a.Body), "issue", "comment", strconv.Itoa(a.Issue), "--body-file", "-")
	case triage.Close:
		_, err = g.run(nil, "issue", "close", strconv.Itoa(a.Issue), "--comment", a.Body)
	}

	return err
}

// run runs gh with the body on stdin, and returns its output.
func (g ghIssues) run(stdin io.Reader, args ...string) ([]byte, error) {
	_, err := exec.LookPath("gh")
	if err != nil {
		return nil, errNoGH
	}

	cmd := exec.CommandContext(g.ctx, "gh", args...) // #nosec G204 -- gh with the triage's own arguments; bodies go on stdin
	cmd.Dir = g.dir
	cmd.Stdin = stdin

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh %s: %w: %s", strings.Join(args[:2], " "), err, strings.TrimSpace(stderr.String()))
	}

	return data, nil
}

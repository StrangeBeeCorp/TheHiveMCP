// Package issues reads and writes the repository's GitHub issues with gh: the nightly findings the
// triage files (internal/harness/triage) and the debt agents file for what is out of their scope.
package issues

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/triage"
)

// ErrNoGH means gh is not there to read or write the issues.
var ErrNoGH = errors.New("gh is needed for the repository's issues (install it and run gh auth login)")

// GH reads and writes issues with gh, in the repository of Dir.
type GH struct {
	Ctx context.Context //nolint:containedctx // one command's calls share its context
	Dir string
}

// Listed is an issue as `gh issue list --json` prints it.
type Listed struct {
	triage.Issue

	URL       string    `json:"url"`
	CreatedAt time.Time `json:"createdAt"`
}

// List returns the open issues labelled label, with their bodies.
func (g GH) List(label string) ([]triage.Issue, error) {
	found, err := g.query(label, "number,title,body")
	if err != nil {
		return nil, err
	}

	out := make([]triage.Issue, 0, len(found))
	for _, l := range found {
		out = append(out, l.Issue)
	}

	return out, nil
}

// Oldest returns the open issues labelled label, the oldest first.
func (g GH) Oldest(label string) ([]Listed, error) {
	found, err := g.query(label, "number,title,url,createdAt")
	if err != nil {
		return nil, err
	}

	slices.SortFunc(found, func(a, b Listed) int { return a.CreatedAt.Compare(b.CreatedAt) })

	return found, nil
}

// Label makes sure a label exists before an issue carries it.
func (g GH) Label(name, color, description string) error {
	_, err := g.Run(nil, "label", "create", name, "--force", "--color", color, "--description", description)

	return err
}

// Create files an issue with one label and returns what gh prints: its URL.
func (g GH) Create(title, label, body string) (string, error) {
	out, err := g.Run(strings.NewReader(body), "issue", "create", "--title", title, "--label", label, "--body-file", "-")

	return strings.TrimSpace(string(out)), err
}

// Apply carries out one of the triage's actions.
func (g GH) Apply(a triage.Action) error {
	var err error

	switch a.Kind {
	case triage.Open:
		_, err = g.Create(a.Title, triage.Label, a.Body)
	case triage.Comment:
		_, err = g.Run(strings.NewReader(a.Body), "issue", "comment", strconv.Itoa(a.Issue), "--body-file", "-")
	case triage.Close:
		_, err = g.Run(nil, "issue", "close", strconv.Itoa(a.Issue), "--comment", a.Body)
	}

	return err
}

// Run runs gh with stdin, and returns its output.
func (g GH) Run(stdin io.Reader, args ...string) ([]byte, error) {
	_, err := exec.LookPath("gh")
	if err != nil {
		return nil, ErrNoGH
	}

	cmd := exec.CommandContext(g.Ctx, "gh", args...) // #nosec G204 -- gh with the harness's own arguments; bodies go on stdin
	cmd.Dir = g.Dir
	cmd.Stdin = stdin

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh %s: %w: %s", strings.Join(args[:2], " "), err, strings.TrimSpace(stderr.String()))
	}

	return data, nil
}

func (g GH) query(label, fields string) ([]Listed, error) {
	data, err := g.Run(nil, "issue", "list", "--label", label, "--state", "open", "--limit", "500", "--json", fields)
	if err != nil {
		return nil, err
	}

	var found []Listed

	err = json.Unmarshal(data, &found)
	if err != nil {
		return nil, fmt.Errorf("read gh's issue list: %w", err)
	}

	return found, nil
}

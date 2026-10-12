// Package evidence records that the gates of the pr cadence passed at one exact commit (make pr),
// decides whether that commit may be pushed, and renders the record for the pull request.
package evidence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"
)

// Dir holds one record per commit that make pr judged, in the checkout (git-ignored).
const Dir = ".cache/harness/evidence"

const (
	dirPerm  = 0o750
	filePerm = 0o600
	shortSHA = 7
)

// Evidence is what make pr found at one commit.
type Evidence struct {
	Head     string          `json:"head"`
	Base     string          `json:"base"`
	Branch   string          `json:"branch"`
	Verdict  runner.Verdict  `json:"verdict"`
	Started  time.Time       `json:"started"`
	Finished time.Time       `json:"finished"`
	Suites   []string        `json:"suites"`
	Changed  []string        `json:"changed"`
	Results  []runner.Result `json:"results"`
	// FirstSnapshot is true when the base had no snapshot of what the model reads: this change
	// creates it, which changes nothing a model reads.
	FirstSnapshot bool `json:"first_snapshot,omitempty"`
	// Against, when set, is the pull request's base: Changed and FirstSnapshot then describe the
	// change against it, while Base is what selected the gates. Only bin/harness evidence --base
	// sets it: make pr's record never holds it.
	Against string `json:"against,omitempty"`
	// Coverage, when set, is how the review that let head ship read the branch at an earlier commit: the
	// push records it, and it lists the commits that review did not read as they are now.
	Coverage *Coverage `json:"coverage,omitempty"`
}

func path(root, head string) string {
	return filepath.Join(root, Dir, head+".json")
}

// Write records the evidence under its commit.
func (ev Evidence) Write(root string) (string, error) {
	p := path(root, ev.Head)

	err := os.MkdirAll(filepath.Dir(p), dirPerm)
	if err != nil {
		return "", fmt.Errorf("create %s: %w", filepath.Dir(p), err)
	}

	data, err := json.MarshalIndent(ev, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode the evidence: %w", err)
	}

	err = os.WriteFile(p, data, filePerm)
	if err != nil {
		return "", fmt.Errorf("write %s: %w", p, err)
	}

	return p, nil
}

// Read returns the evidence of a commit, or nil when make pr never judged it.
func Read(root, head string) (*Evidence, error) {
	data, err := os.ReadFile(path(root, head)) // #nosec G304 -- this checkout's own record, named by a commit id
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil //nolint:nilnil // no record is not an error
	}

	if err != nil {
		return nil, fmt.Errorf("read the evidence of %s: %w", short(head), err)
	}

	var ev Evidence

	err = json.Unmarshal(data, &ev)
	if err != nil {
		return nil, fmt.Errorf("decode the evidence of %s: %w", short(head), err)
	}

	return &ev, nil
}

// ErrNotShippable is why a commit may not be pushed.
var ErrNotShippable = errors.New("not ready to push")

// PushAllowed decides whether head may be pushed. The pr gates must have passed at head (make
// pr), every one of gates with a result, and the adversarial review must be ready for head.
// Strict (anyone but a person who passed --person) refuses; otherwise the problems come back as
// warnings, since a person can push anyway.
func PushAllowed(head string, ev *Evidence, review *Review, gates []string, strict bool) ([]string, error) {
	var problems []string

	switch {
	case ev == nil || ev.Head != head:
		problems = append(problems, fmt.Sprintf("the pr gates have not run at %s: run make pr", short(head)))
	case ev.Verdict != runner.Pass:
		problems = append(problems, fmt.Sprintf("the pr gates were %s at %s: fix them, commit, and run make pr", ev.Verdict, short(head)))
	default:
		if missing := ev.missing(gates); len(missing) > 0 {
			problems = append(problems, fmt.Sprintf("the evidence of %s has no result for %v: run make pr", short(head), missing))
		}
	}

	switch {
	case review == nil || review.Verdict == "":
		problems = append(problems, "no adversarial review of this branch: run the review in fix mode")
	case review.Head != head:
		problems = append(problems, fmt.Sprintf("the review judged %s but HEAD is %s: let the review check the new commits",
			short(review.Head), short(head)))
	case !review.Ready():
		problems = append(problems, fmt.Sprintf("the review's verdict is %s: finish the review first", review.Verdict))
	}

	if strict && len(problems) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrNotShippable, problems[0])
	}

	return problems, nil
}

// missing lists the gates the evidence has no result for.
func (ev Evidence) missing(gates []string) []string {
	have := map[string]bool{}
	for _, r := range ev.Results {
		have[r.Gate] = true
	}

	var out []string

	for _, g := range gates {
		if !have[g] {
			out = append(out, g)
		}
	}

	return out
}

func short(sha string) string {
	if len(sha) > shortSHA {
		return sha[:shortSHA]
	}

	if sha == "" {
		return "(none)"
	}

	return sha
}

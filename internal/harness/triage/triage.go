// Package triage turns a night's results into findings: one GitHub issue per gate and shard that
// fails or cannot run, kept open while it does, and closed once it passes again.
package triage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"
)

const (
	// Label marks the issues the nightly triage owns: it opens, comments and closes only those.
	Label = "nightly"
	// logLines and logBytes bound the log tail an issue or a comment quotes.
	logLines = 40
	logBytes = 4000
	// anyGate is the gate of a shard's own finding: the job recorded no result at all.
	anyGate = "*"
)

// keyMarker is how an issue names the finding it tracks, in a comment GitHub does not render.
var keyMarker = regexp.MustCompile(`<!-- nightly-finding: (\S+) -->`)

// Outcome is what one gate did in one shard of the night.
type Outcome struct {
	Shard   string
	Gate    string
	Verdict runner.Verdict
	Summary string
	// Log is the tail of the gate's log, when the shard uploaded it.
	Log string
}

// Key names the finding: the gate in its shard (the two integration legs are two findings).
func (o Outcome) Key() string { return o.Shard + "/" + o.Gate }

// failing says whether the outcome is a finding: a failure, or a gate that could not run.
func (o Outcome) failing() bool {
	return o.Verdict == runner.Fail || o.Verdict == runner.Unavailable
}

// Issue is an open issue labelled nightly.
type Issue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

// Key is the finding the issue tracks, or "" when its body names none.
func (i Issue) Key() string {
	m := keyMarker.FindStringSubmatch(i.Body)
	if m == nil {
		return ""
	}

	return m[1]
}

// Kind is what the triage does with an issue.
type Kind string

const (
	// Open files a new finding.
	Open Kind = "open"
	// Comment adds a night to a finding still open.
	Comment Kind = "comment"
	// Close closes a finding whose gate passed.
	Close Kind = "close"
)

// Action is one change to the issues.
type Action struct {
	Kind  Kind
	Issue int // the issue commented or closed
	Title string
	Body  string
}

// Plan is what the night's outcomes do to the open findings: a failing gate with no issue opens
// one, a failing gate with one comments on it, and a passing gate closes its own. A gate that was
// skipped, or did not run tonight, leaves its issue as it is.
func Plan(outcomes []Outcome, open []Issue, runURL string) []Action {
	byKey := map[string]Issue{}

	for _, i := range open {
		if k := i.Key(); k != "" {
			byKey[k] = i
		}
	}

	var actions []Action

	for _, o := range merge(outcomes) {
		issue, tracked := byKey[o.Key()]

		switch {
		case o.failing() && tracked:
			actions = append(actions, Action{Kind: Comment, Issue: issue.Number, Body: "Still " + state(o) + " in " + runURL + ".\n\n" + details(o)})
		case o.failing():
			actions = append(actions, Action{Kind: Open, Title: title(o), Body: fmt.Sprintf("<!-- nightly-finding: %s -->\n%s since %s.\n\n%s", o.Key(), sentence(o), runURL, details(o))})
		case o.Verdict == runner.Pass && tracked:
			actions = append(actions, Action{Kind: Close, Issue: issue.Number, Body: "Passes again in " + runURL + "."})
		}
	}

	return actions
}

// merge keeps one outcome per key, in key order: the worst, when a gate ran twice in a shard.
func merge(outcomes []Outcome) []Outcome {
	worse := []runner.Verdict{runner.Skipped, runner.Pass, runner.Unavailable, runner.Fail}
	byKey := map[string]Outcome{}

	for _, o := range outcomes {
		if prev, ok := byKey[o.Key()]; !ok || slices.Index(worse, o.Verdict) > slices.Index(worse, prev.Verdict) {
			byKey[o.Key()] = o
		}
	}

	keys := slices.Sorted(maps.Keys(byKey))

	merged := make([]Outcome, 0, len(keys))
	for _, k := range keys {
		merged = append(merged, byKey[k])
	}

	return merged
}

func state(o Outcome) string {
	if o.Verdict == runner.Unavailable {
		return "unable to run"
	}

	return "failing"
}

func title(o Outcome) string {
	what := "fails"
	if o.Verdict == runner.Unavailable {
		what = "cannot run"
	}

	if o.Gate == anyGate {
		return fmt.Sprintf("nightly: the %s shard records no result", o.Shard)
	}

	return fmt.Sprintf("nightly: %s %s (%s)", o.Gate, what, o.Shard)
}

func sentence(o Outcome) string {
	if o.Gate == anyGate {
		return fmt.Sprintf("The %s shard of the nightly workflow recorded no result: its job failed before the harness ran", o.Shard)
	}

	return fmt.Sprintf("The %s gate is %s in the %s shard of the nightly workflow", o.Gate, state(o), o.Shard)
}

// details quotes the gate's summary and the tail of its log, fenced so that nothing in them is
// rendered: a log is data.
func details(o Outcome) string {
	if o.Gate == anyGate {
		return fence("summary", o.Summary) + "\nOpen the run's job log: the step that failed is before `bin/harness run`.\n"
	}

	var b strings.Builder

	b.WriteString(fence("summary", o.Summary))

	if o.Log != "" {
		b.WriteString("\n")
		b.WriteString(fence("log (last lines)", o.Log))
	}

	b.WriteString("\nThe run's `" + o.Shard + "` artifact holds the whole log (a fuzz input it found is in the `fuzz-inputs` artifact, under testdata/fuzz). " +
		"Run it again with `make gate ID=" + o.Gate + " CADENCE=nightly`; `make next` lists the open findings.\n")

	return b.String()
}

// fence quotes text in a code block whose fence no backtick run inside it can close.
func fence(label, text string) string {
	f := "```"
	for strings.Contains(text, f) {
		f += "`"
	}

	return label + ":\n\n" + f + "text\n" + strings.TrimRight(text, "\n") + "\n" + f + "\n"
}

// ErrNoShards means the directory holds no shard.
var ErrNoShards = errors.New("no shard")

// Read reads the night's outcomes from dir, where each subdirectory is one shard's artifact: the
// .cache/harness/runs directories of its job. A shard of expect with no results.json gets one
// finding of its own, so that a job that failed before the harness ran is not silent.
func Read(dir string, expect []string) ([]Outcome, error) {
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}

	seen := map[string]bool{}

	var outcomes []Outcome

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		found, err := readShard(filepath.Join(dir, e.Name()), e.Name())
		if err != nil {
			return nil, err
		}

		if len(found) > 0 {
			seen[e.Name()] = true
			// The shard ran: its own finding, if one is open, closes.
			outcomes = append(outcomes, Outcome{Shard: e.Name(), Gate: anyGate, Verdict: runner.Pass})
		}

		outcomes = append(outcomes, found...)
	}

	for _, s := range expect {
		if !seen[s] {
			outcomes = append(outcomes, Outcome{Shard: s, Gate: anyGate, Verdict: runner.Unavailable, Summary: "no results.json in the shard's artifact"})
		}
	}

	if len(outcomes) == 0 {
		return nil, fmt.Errorf("%w in %s, and none expected", ErrNoShards, dir)
	}

	return outcomes, nil
}

// readShard reads every results.json under a shard's artifact, through an os.Root: a symlink in
// the artifact cannot lead outside it.
func readShard(dir, shard string) ([]Outcome, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("shard %s: %w", shard, err)
	}
	defer func() { _ = root.Close() }()

	var outcomes []Outcome

	err = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || d.Name() != "results.json" {
			return nil
		}

		data, err := root.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}

		var rec runner.Record

		err = json.Unmarshal(data, &rec)
		if err != nil {
			return fmt.Errorf("parse %s: %w", p, err)
		}

		for _, r := range rec.Results {
			outcomes = append(outcomes, Outcome{Shard: shard, Gate: r.Gate, Verdict: r.Verdict, Summary: r.Summary, Log: logTail(root, path.Dir(p), r.Log)})
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("shard %s: %w", shard, err)
	}

	return outcomes, nil
}

// logTail is the end of a gate's log: the runner recorded its path on the job's machine, and the
// artifact holds it next to results.json.
func logTail(root *os.Root, dir, recorded string) string {
	if recorded == "" {
		return ""
	}

	data, err := root.ReadFile(path.Join(dir, filepath.Base(recorded)))
	if err != nil {
		return ""
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > logLines {
		lines = lines[len(lines)-logLines:]
	}

	tail := strings.Join(lines, "\n")
	if len(tail) > logBytes {
		tail = "…" + tail[len(tail)-logBytes:]
	}

	return tail
}

package thresholds

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/dashboard"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/triage"
)

// fakeIssues keeps the metrics issues in memory, as GitHub would.
type fakeIssues struct {
	open    []triage.Issue
	labels  []string
	applied []triage.Action
	next    int
}

func (f *fakeIssues) List(label string) ([]triage.Issue, error) {
	if label != Label {
		return nil, nil
	}

	return append([]triage.Issue(nil), f.open...), nil
}

func (f *fakeIssues) Label(name, _, _ string) error {
	f.labels = append(f.labels, name)
	return nil
}

func (f *fakeIssues) ApplyAs(a triage.Action, label string) error {
	if label != Label {
		return nil
	}

	f.applied = append(f.applied, a)

	switch a.Kind {
	case triage.Open:
		f.next++
		f.open = append(f.open, triage.Issue{Number: f.next, Title: a.Title, Body: a.Body})
	case triage.Close:
		for i, o := range f.open {
			if o.Number == a.Issue {
				f.open = append(f.open[:i], f.open[i+1:]...)
			}
		}
	}

	return nil
}

// repo is a checkout with the repository's thresholds and, for coverage, a night's log.
func repo(t *testing.T) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "..", Path))
	require.NoError(t, err)

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "gates"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(root, Path), data, 0o600)) // #nosec G703 -- the test's own directory

	return root
}

func night(t *testing.T, coverage float64) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "night", "checks", "runs", "nightly-1")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "coverage.log"),
		[]byte("ok\ntotal:\t\t(statements)\t\t"+strconv.FormatFloat(coverage, 'f', 1, 64)+"%\n"), 0o600))

	return filepath.Dir(filepath.Dir(filepath.Dir(dir)))
}

// A threshold crossed on a night files one metrics issue, the night after it is still crossed
// comments on it, and the first night under it closes it: what DL-8158 is done for. Each night
// reads the values the night before wrote.
func TestAThresholdCrossedFilesOneIssueAndTheNightUnderClosesIt(t *testing.T) {
	t.Parallel()

	root := repo(t)
	gh := &fakeIssues{}
	now := time.Date(2026, 10, 8, 3, 0, 0, 0, time.UTC)
	data := dashboard.Data{Now: now, Tags: []dashboard.Tag{{Name: "v1", When: now.AddDate(0, 0, -3)}}}
	values := filepath.Join(t.TempDir(), "metrics.json")

	run := func(coverage float64, previous string) string {
		var out bytes.Buffer

		require.NoError(t, Run(Night{Root: root, Data: data, Runs: night(t, coverage), Previous: previous, Out: values, RunURL: "RUN", Issues: gh}, &out))

		return out.String()
	}

	run(64.8, "")
	assert.Empty(t, gh.applied, "the first night has nothing to compare coverage with")

	saved := filepath.Join(t.TempDir(), "previous.json")
	copyFile(t, values, saved)

	out := run(61.0, saved)

	require.Len(t, gh.open, 1)
	assert.Equal(t, "metrics: coverage-drop past its threshold", gh.open[0].Title)
	assert.Contains(t, gh.open[0].Body, "<!-- metrics-threshold: coverage-drop -->")
	assert.Contains(t, gh.open[0].Body, "coverage is 61.0, 64.8 the night before: a drop of 3.8, for at most 2.0, in RUN.")
	assert.Contains(t, gh.open[0].Body, "tests were removed")
	assert.Equal(t, []string{Label}, gh.labels)
	assert.Contains(t, out, "FAIL     coverage-drop")

	copyFile(t, values, saved)
	run(58.0, saved)
	require.Len(t, gh.open, 1, "still crossed: one issue, commented on")
	assert.Equal(t, triage.Comment, gh.applied[len(gh.applied)-1].Kind)

	copyFile(t, values, saved)
	run(58.5, saved)
	assert.Empty(t, gh.open, "back under: closed")
	assert.Equal(t, triage.Close, gh.applied[len(gh.applied)-1].Kind)
}

// A dry run prints what would change and changes nothing; an issue the thresholds do not own is
// left alone, and a metric that could not be measured comments on its own issue only.
func TestADryRunChangesNothingAndForeignIssuesAreLeftAlone(t *testing.T) {
	t.Parallel()

	gh := &fakeIssues{open: []triage.Issue{
		{Number: 3, Body: "<!-- nightly-finding: checks/lint -->"},
		{Number: 4, Body: "<!-- metrics-threshold: finding-age -->"},
	}}
	data := dashboard.Data{Now: time.Now(), Missing: map[string]string{dashboard.SourceGitHub: "gh is not installed"}}

	var out bytes.Buffer

	require.NoError(t, Run(Night{Root: repo(t), Data: data, Out: filepath.Join(t.TempDir(), "m.json"), RunURL: "RUN", Dry: true, Issues: gh}, &out))
	assert.Empty(t, gh.applied)
	assert.Contains(t, out.String(), "UNAVAILABLE finding-age: oldest_finding_days could not be measured tonight")
	assert.Contains(t, out.String(), "comment #4")
	assert.NotContains(t, out.String(), "#3")
}

// Tonight's values are written without the unknown ones (JSON has no NaN), and read back.
func TestTonightsValuesAreWrittenForTheNextNight(t *testing.T) {
	t.Parallel()

	p := filepath.Join(t.TempDir(), "dir", "metrics.json")
	require.NoError(t, writeValues(p, map[string]Value{
		Coverage: {Now: 64.8, Baseline: math.NaN()}, CIMinutes: {Now: 10, Baseline: 9}, OldestFindingDays: {Now: math.NaN()},
	}))

	data, err := os.ReadFile(p) // #nosec G304 -- the test's own file
	require.NoError(t, err)

	var v map[string]Value
	require.NoError(t, json.Unmarshal(data, &v))
	assert.Equal(t, map[string]Value{Coverage: {Now: 64.8}, CIMinutes: {Now: 10, Baseline: 9}}, v)

	read, err := readValues(p)
	require.NoError(t, err)
	assert.Equal(t, v, read)

	read, err = readValues(filepath.Join(t.TempDir(), "absent.json"))
	require.NoError(t, err, "the first night has none")
	assert.Empty(t, read)

	bad := filepath.Join(t.TempDir(), "bad.json")
	require.NoError(t, os.WriteFile(bad, []byte("{"), 0o600))

	_, err = readValues(bad)
	require.Error(t, err)
}

// The command needs the run's URL; a stray argument is a usage error.
func TestTheCommandLineIsChecked(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{}, {"--run-url", "u", "extra"}, {"--nope"}} {
		require.ErrorIs(t, Command(t.Context(), t.TempDir(), args, &bytes.Buffer{}), ErrUsage, strings.Join(args, " "))
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()

	data, err := os.ReadFile(from) // #nosec G304 -- the test's own file
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(to, data, 0o600)) // #nosec G703 -- the test's own file
}

package thresholds

import (
	"bytes"
	"encoding/json"
	"errors"
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

var errGitHub = errors.New("HTTP 502: bad gateway")

// runURL is the night every test run is said to come from.
const runURL = "RUN"

// fakeIssues keeps the metrics issues in memory, as GitHub would, or fails every change.
type fakeIssues struct {
	fail    bool
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
	if f.fail {
		return errGitHub
	}

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

// A threshold crossed on a night files one metrics issue, a night it is still crossed comments on
// it, and the first night under it closes it: what DL-8158 is done for. Each night reads the values
// the night before wrote; a drop stays crossed until the metric is back within its limit of the
// level it fell from, not merely once it stops falling.
func TestAThresholdCrossedFilesOneIssueAndTheNightUnderClosesIt(t *testing.T) {
	t.Parallel()

	root := repo(t)
	gh := &fakeIssues{}
	now := time.Date(2026, 10, 8, 3, 0, 0, 0, time.UTC)
	data := dashboard.Data{Now: now, Tags: []dashboard.Tag{{Name: "v1", When: now.AddDate(0, 0, -3)}}}
	values := filepath.Join(t.TempDir(), "metrics.json")

	run := func(coverage float64, previous string) string {
		var out bytes.Buffer

		require.NoError(t, Run(Night{Root: root, Data: data, Runs: night(t, coverage), Previous: previous, Out: values, RunURL: runURL, Issues: gh}, &out))

		return out.String()
	}

	run(64.8, "")
	assert.Empty(t, gh.applied, "the first night has nothing to compare coverage with, and files nothing")

	saved := filepath.Join(t.TempDir(), "previous.json")
	copyFile(t, values, saved)

	out := run(61.0, saved)

	require.Len(t, gh.open, 1)
	assert.Equal(t, "metrics: coverage-drop past its threshold", gh.open[0].Title)
	assert.Contains(t, gh.open[0].Body, "<!-- metrics-threshold: coverage-drop -->")
	assert.Contains(t, gh.open[0].Body, "coverage is 61.0, from 64.8: a drop of 3.8, for at most 2.0, in RUN.")
	assert.Contains(t, gh.open[0].Body, "tests were removed")
	assert.Equal(t, []string{Label}, gh.labels)
	assert.Contains(t, out, "FAIL     coverage-drop")

	copyFile(t, values, saved)
	run(61.0, saved)
	require.Len(t, gh.open, 1, "no further drop, but still 3.8 under the level it fell from: one issue, commented on")
	assert.Equal(t, triage.Comment, gh.applied[len(gh.applied)-1].Kind)

	copyFile(t, values, saved)
	run(63.0, saved)
	assert.Empty(t, gh.open, "back within 2 points of 64.8: closed")
	assert.Equal(t, triage.Close, gh.applied[len(gh.applied)-1].Kind)
}

// A night with nothing to compare with comments on an open issue and closes none, and a night that
// could not measure a metric keeps the last night's value of it for the next.
func TestANightWithNothingToCompareClosesNothing(t *testing.T) {
	t.Parallel()

	gh := &fakeIssues{open: []triage.Issue{{Number: 9, Body: "<!-- metrics-threshold: coverage-drop -->"}}}
	values := filepath.Join(t.TempDir(), "metrics.json")
	data := dashboard.Data{Now: time.Now(), Local: dashboard.LocalRun{Coverage: -1}}

	require.NoError(t, Run(Night{Root: repo(t), Data: data, Runs: night(t, 60), Out: values, RunURL: runURL, Issues: gh}, &bytes.Buffer{}))
	require.Len(t, gh.open, 1, "no previous night: not closed")
	assert.Equal(t, triage.Comment, gh.applied[0].Kind)

	previous := filepath.Join(t.TempDir(), "previous.json")
	require.NoError(t, writeValues(previous, map[string]Value{Coverage: {Now: 61, Reference: 64.8}}))
	require.NoError(t, Run(Night{Root: repo(t), Data: data, Runs: t.TempDir(), Previous: previous, Out: values, RunURL: runURL, Issues: gh}, &bytes.Buffer{}))

	kept, err := readValues(values)
	require.NoError(t, err)
	assert.Equal(t, Value{Now: 61, Reference: 64.8}, kept[Coverage], "no coverage log tonight: the last night's level is kept")
}

// A night whose issues could not be changed leaves no values: the next night compares with the
// last one that did, so a crossing it could not file is filed then.
func TestANightThatCouldNotFileLeavesNoValues(t *testing.T) {
	t.Parallel()

	previous := filepath.Join(t.TempDir(), "previous.json")
	require.NoError(t, writeValues(previous, map[string]Value{Coverage: {Now: 64.8}}))

	values := filepath.Join(t.TempDir(), "metrics.json")
	gh := &fakeIssues{fail: true}

	err := Run(Night{Root: repo(t), Data: dashboard.Data{Now: time.Now()}, Runs: night(t, 60), Previous: previous, Out: values, RunURL: runURL, Issues: gh}, &bytes.Buffer{})
	require.Error(t, err)
	assert.NoFileExists(t, values)
}

// Tonight's values: a metric whose source was not read is unknown, not 0; the coverage is the
// night's log's, this clone's last make pr's only when no night was given.
func TestTonightsValuesAreUnknownWhenTheirSourceWasNotRead(t *testing.T) {
	t.Parallel()

	now := time.Now()
	issuesUnread := dashboard.Data{Now: now, Missing: map[string]string{dashboard.SourceIssues: "gh issue list failed"}}
	assert.True(t, math.IsNaN(Values(issuesUnread, "")[OldestFindingDays].Now), "the issues were not read")
	assert.Zero(t, Values(dashboard.Data{Now: now}, "")[OldestFindingDays].Now, "read, and none open")

	local := dashboard.Data{Now: now, Local: dashboard.LocalRun{Coverage: 66.2}}
	assert.InDelta(t, 66.2, Values(local, "")[Coverage].Now, 0.01, "no night: this clone's last make pr")
	assert.True(t, math.IsNaN(Values(local, t.TempDir())[Coverage].Now), "a night without a coverage log: unknown, not the local run's")
	assert.InDelta(t, 60, Values(local, night(t, 60))[Coverage].Now, 0.01)
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

	require.NoError(t, Run(Night{Root: repo(t), Data: data, Out: filepath.Join(t.TempDir(), "m.json"), RunURL: runURL, Dry: true, Issues: gh}, &out))
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

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"
)

const (
	changelogRegistry = `
scopes: {edit: a, stop: a, commit: a, pr: a, ci: a, nightly: a}
suites:
  - {id: everything, paths: [gates/**]}
  - {id: surface, paths: [baselines/**]}
  - {id: docs, paths: ["*.md"]}
gates:
  - {id: changelog, builtin: changelog, cadence: [ci], suites: [surface], budget_s: 10, blocks: x}
`
	snapshotSummary = "baselines/surface/tokens.json"
	// ignoreFile and ignoreCache keep the harness's cache out of the test repositories.
	ignoreFile    = ".gitignore"
	ignoreCache   = ".cache/\n"
	emptyJSON     = "{}\n"
	snapshotTool  = "baselines/surface/tools/search-entities.json"
	changelogMD   = "CHANGELOG.md"
	baseChangelog = "# Changelog\n\n## [Unreleased]\n\n## [1.0.0]\n\n- The first release.\n"
)

// surfaceCheckout is a repository whose main branch has a snapshot of what the model reads (or
// not, the first time) and a changelog, with a work branch checked out.
func surfaceCheckout(t *testing.T, withSnapshot bool) string {
	t.Helper()

	files := map[string]string{
		registryPath: changelogRegistry, limitsPath: glueLimits, ignoreFile: ignoreCache, changelogMD: baseChangelog,
	}
	if withSnapshot {
		files[snapshotSummary], files[snapshotTool] = emptyJSON, emptyJSON
	}

	dir := gittest.Repo(t, files)
	gittest.Git(t, dir, "checkout", "-q", "-b", "work")
	t.Chdir(dir)
	t.Setenv("CI", "")
	t.Setenv("HARNESS_BASE", "")

	return dir
}

func commitAll(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	gittest.Write(t, dir, files)
	gittest.Git(t, dir, "add", "-A")
	gittest.Commit(t, dir)
}

// A change to the snapshot needs a new entry under ## [Unreleased]: an edit of an older release
// does not count, and the first snapshot needs none.
//
//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestTheChangelogGateWantsAnUnreleasedEntry(t *testing.T) {
	dir := surfaceCheckout(t, true)
	commitAll(t, dir, map[string]string{snapshotTool: `{"description": "changed"}` + "\n"})

	out, err := harness(t, "run", "--cadence", "ci", "--base", "main")
	require.ErrorIs(t, err, errFailed, out)
	assert.Equal(t, runner.Fail, lastRun(t, dir).Verdict)

	commitAll(t, dir, map[string]string{changelogMD: baseChangelog + "- The first release, reworded.\n"})
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".cache")))

	_, err = harness(t, "run", "--cadence", "ci", "--base", "main")
	require.ErrorIs(t, err, errFailed, "an older release's edit is no entry")

	commitAll(t, dir, map[string]string{changelogMD: "# Changelog\n\n## [Unreleased]\n\n- search-entities says more.\n\n## [1.0.0]\n\n- The first release.\n"})
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".cache")))

	out, err = harness(t, "run", "--cadence", "ci", "--base", "main")
	require.NoError(t, err, out)
	assert.Equal(t, runner.Pass, lastRun(t, dir).Verdict)
}

//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestTheFirstSnapshotNeedsNoChangelogEntry(t *testing.T) {
	dir := surfaceCheckout(t, false)
	commitAll(t, dir, map[string]string{snapshotSummary: emptyJSON, snapshotTool: emptyJSON})

	out, err := harness(t, "run", "--cadence", "ci", "--base", "main")
	require.NoError(t, err, out)
	assert.Equal(t, runner.Pass, lastRun(t, dir).Verdict)
}

// Without a base there is no change to judge: the gate is skipped, with why, never a pass, and
// `check changelog` judges the change since the base it is given.
//
//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestTheChangelogGateJudgesOnlyAChangeWithABase(t *testing.T) {
	dir := surfaceCheckout(t, true)
	commitAll(t, dir, map[string]string{snapshotTool: `{"description": "changed"}` + "\n"})

	out, err := harness(t, "run", "--cadence", "ci", "--all")
	require.NoError(t, err, out)

	rec := lastRun(t, dir)
	require.Len(t, rec.Results, 1)
	assert.Equal(t, runner.Skipped, rec.Results[0].Verdict)
	assert.Contains(t, rec.Results[0].Summary, "no base to compare with")

	out, err = harness(t, "check", "changelog", "--base", "main")
	require.ErrorIs(t, err, errFailed)
	assert.Contains(t, out, "CHANGELOG.md did not")

	out, err = harness(t, "check", "changelog")
	require.ErrorIs(t, err, errIncomplete, "no origin/main to compare with")
	assert.Contains(t, out, "changelog: not judged")

	_, err = harness(t, "check", "spec", "--base", "main")

	var usage usageError
	require.ErrorAs(t, err, &usage, "only the changelog check takes a base")
}

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	docA     = "docs/a.md"
	editedA  = "# a, edited\n"
	toolGate = "tool-tests"
	goFile   = "internal/x.go"
)

// glueRegistry has a docs suite and a tools suite, and a gate for each: the docs gate passes and
// the tools gate fails, so a run shows which gates the change selected.
const glueRegistry = `
scopes: {edit: a, stop: a, commit: a, pr: a, ci: a, nightly: a}
suites:
  - {id: everything, paths: [gates/**]}
  - {id: tools, paths: [internal/**]}
  - {id: docs, paths: [docs/**]}
gates:
  - {id: docs-check, runs: "true", cadence: [ci], suites: [docs], budget_s: 10, blocks: x}
  - {id: tool-tests, runs: "echo tool tests broken; exit 1", cadence: [ci], suites: [tools], budget_s: 10, blocks: x}
`

// checkout is a repository whose main branch has the registry above, a doc and a Go file, with
// a work branch checked out. It becomes the working directory, where the harness finds its
// repository, and CI and HARNESS_BASE are cleared so each test sets what it compares with.
func checkout(t *testing.T) string {
	t.Helper()

	dir := gittest.Repo(t, map[string]string{
		registryPath: glueRegistry, limitsPath: "{}\n", ".gitignore": ".cache/\n",
		docA: "# a\n", goFile: "package x\n",
	})
	gittest.Git(t, dir, "checkout", "-q", "-b", "work")
	t.Chdir(dir)
	t.Setenv("CI", "")
	t.Setenv("HARNESS_BASE", "")

	return dir
}

func harness(t *testing.T, args ...string) (string, error) {
	t.Helper()

	var out strings.Builder

	err := dispatch(t.Context(), args, &out)

	return out.String(), err
}

// lastRun reads the record of the run the harness just made.
func lastRun(t *testing.T, dir string) runner.Record {
	t.Helper()

	files, err := filepath.Glob(filepath.Join(dir, ".cache", "harness", "runs", "*", "results.json"))
	require.NoError(t, err)
	require.Len(t, files, 1)

	data, err := os.ReadFile(files[0])
	require.NoError(t, err)

	var rec runner.Record
	require.NoError(t, json.Unmarshal(data, &rec))

	return rec
}

func verdicts(rec runner.Record) map[string]runner.Verdict {
	out := map[string]runner.Verdict{}
	for _, r := range rec.Results {
		out[r.Gate] = r.Verdict
	}

	return out
}

// A docs-only change runs the docs gates and skips the others, which still get a result.
//
//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestADocsOnlyChangeRunsOnlyTheGatesItSelects(t *testing.T) {
	dir := checkout(t)
	gittest.Write(t, dir, map[string]string{docA: editedA})

	out, err := harness(t, "select", "--base", "main")
	require.NoError(t, err)
	assert.Contains(t, out, "docs")
	assert.NotContains(t, out, "tools")

	out, err = harness(t, "run", "--cadence", "ci", "--base", "main")
	require.NoError(t, err, out)
	assert.Contains(t, out, "harness · ci · suites: docs · 1 gate(s)")

	rec := lastRun(t, dir)
	assert.Equal(t, runner.Pass, rec.Verdict)
	assert.Equal(t, map[string]runner.Verdict{"docs-check": runner.Pass, toolGate: runner.Skipped}, verdicts(rec))
}

// A code change runs the code's gates, and a failing one fails the run.
//
//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestACodeChangeRunsItsGatesAndTheirFailureFailsTheRun(t *testing.T) {
	dir := checkout(t)
	gittest.Write(t, dir, map[string]string{goFile: "package x // edited\n"})
	gittest.Git(t, dir, "add", "-A")
	gittest.Commit(t, dir)

	out, err := harness(t, "run", "--cadence", "ci", "--base", "main")
	require.ErrorIs(t, err, errFailed)
	assert.Contains(t, out, "tool tests broken", "the failing gate's output is shown")

	rec := lastRun(t, dir)
	assert.Equal(t, runner.Fail, rec.Verdict)
	assert.Equal(t, map[string]runner.Verdict{"docs-check": runner.Skipped, toolGate: runner.Fail}, verdicts(rec))
}

// With nothing to compare with, or a path no suite claims, every gate runs.
func TestWithoutAComparisonEveryGateRuns(t *testing.T) {
	dir := checkout(t)

	out, err := harness(t, "select")
	require.NoError(t, err)
	assert.Equal(t, "every suite: no origin/main to compare with\n", out, "the default base is missing")

	t.Setenv("CI", "true")

	out, err = harness(t, "select")
	require.NoError(t, err)
	assert.Equal(t, "every suite: CI without a base: a push, not a pull request\n", out)

	out, err = harness(t, "select", "--all")
	require.NoError(t, err)
	assert.Equal(t, "every suite: asked for every gate\n", out)

	gittest.Write(t, dir, map[string]string{"weird.txt": "x\n"})

	out, err = harness(t, "select", "--base", "main")
	require.NoError(t, err)
	assert.Equal(t, "every suite: a path no suite claims: weird.txt\n", out)
}

// HARNESS_BASE, which CI sets to the pull request's base, is the default comparison.
func TestHarnessBaseIsTheDefaultComparison(t *testing.T) {
	dir := checkout(t)
	gittest.Write(t, dir, map[string]string{docA: editedA})
	t.Setenv("CI", "true")
	t.Setenv("HARNESS_BASE", "main")

	out, err := harness(t, "select")
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%-12s %s\n", "docs", docA), out)
}

// The pre-commit hook selects from what is staged, not from the rest of the working tree.
//
//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestThePreCommitHookSelectsFromTheStagedFiles(t *testing.T) {
	dir := checkout(t)
	gittest.Write(t, dir, map[string]string{docA: "# staged\n"})
	gittest.Git(t, dir, "add", docA)
	gittest.Write(t, dir, map[string]string{goFile: "package x // not staged\n"})

	out, err := harness(t, "select", "--staged")
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%-12s %s\n", "docs", docA), out)
}

// A gate named on the command line runs only when the change selects it, unless --force.
//
//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestANamedGateRunsWhenSelectedOrForced(t *testing.T) {
	dir := checkout(t)
	gittest.Write(t, dir, map[string]string{docA: editedA})

	out, err := harness(t, "run", "--cadence", "ci", "--base", "main", "--gate", toolGate)
	require.NoError(t, err, out)
	assert.Contains(t, out, "0 gate(s)")

	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".cache")))

	_, err = harness(t, "run", "--cadence", "ci", "--base", "main", "--gate", toolGate, "--force")
	require.ErrorIs(t, err, errFailed)
	assert.Equal(t, map[string]runner.Verdict{toolGate: runner.Fail}, verdicts(lastRun(t, dir)))

	_, err = harness(t, "run", "--cadence", "ci", "--gate", "ghost")
	require.ErrorAs(t, err, &usageError{})

	_, err = harness(t, "run", "--cadence", "weekly")
	require.ErrorAs(t, err, &usageError{})
}

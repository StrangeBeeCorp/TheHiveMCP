package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"
)

const (
	integrityRegistry = `
scopes: {edit: a, stop: a, commit: a, pr: a, ci: a, nightly: a}
suites:
  - {id: everything, review: high, paths: [gates/**]}
  - {id: tests, review: high, paths: ["**/*_test.go"]}
gates:
  - {id: integrity, builtin: integrity, cadence: [ci], suites: [tests], budget_s: 10, blocks: x}
`
	searchTestPath = "pkg/search_test.go"
	searchTest     = "package pkg\n\nimport \"testing\"\n\nfunc TestSearch(t *testing.T) {\n\tif broken() {\n\t\tt.Fatal(\"broken\")\n\t}\n}\n"
)

// The integrity gate compares the changed tests with those at the base. Without a base there is
// nothing to compare: the gate is skipped, with why, never a pass, and `check integrity` compares
// with the base it is given.
func TestTheIntegrityGateComparesOnlyWithABase(t *testing.T) {
	dir := gittest.Repo(t, map[string]string{
		registryPath: integrityRegistry, limitsPath: glueLimits, ignoreFile: ignoreCache, searchTestPath: searchTest,
	})
	gittest.Git(t, dir, "checkout", "-q", "-b", "work")
	t.Chdir(dir)
	t.Setenv("CI", "")
	t.Setenv("HARNESS_BASE", "")
	commitAll(t, dir, map[string]string{searchTestPath: strings.Replace(searchTest, "\t\tt.Fatal(\"broken\")\n", "", 1)})

	out, err := harness(t, "run", "--cadence", "ci", "--all")
	require.NoError(t, err, out)

	rec := lastRun(t, dir)
	require.Len(t, rec.Results, 1)
	assert.Equal(t, runner.Skipped, rec.Results[0].Verdict)
	assert.Contains(t, rec.Results[0].Summary, "no base to compare with")

	out, err = harness(t, "check", "integrity", "--base", "main")
	require.ErrorIs(t, err, errFailed, out)
	assert.Contains(t, out, "TestSearch checks less: 0 assertions, was 1")

	out, err = harness(t, "check", "integrity")
	require.ErrorIs(t, err, errIncomplete, "no origin/main to compare with")
	assert.Contains(t, out, "integrity: not judged")
}

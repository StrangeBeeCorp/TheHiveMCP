package main

import (
	"bytes"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"
)

const (
	makefile       = "Makefile"
	baseFlag       = "--base"
	mutationConfig = "tool: example.com/gremlins@v0.0.1\ntimeout_coefficient: 1\nfloors: {pkg/a: 80}\n"
	// gremlinsStub stands in for gremlins: the go target writes report.json where gremlins writes pkg/a's.
	gremlinsStub = "go:\n\tmkdir -p .cache/harness/mutation\n\tcp report.json .cache/harness/mutation/pkg_a.json\n"
	aKilled      = `{"test_efficacy": 90, "mutants_killed": 9, "mutants_lived": 1}`
	aLived       = `{"test_efficacy": 50, "mutants_killed": 1, "mutants_lived": 1}`
	// aTimedOut: 37 mutants found, 30 timed out, as on a machine too busy to measure.
	aTimedOut = `{"test_efficacy": 100, "mutants_killed": 7, "mutants_lived": 0, "mutator_statistics": {"CONDITIONALS_NEGATION": 37}}`
)

// mutationRepo is a checkout whose branch work changes pkg/a, which has a floor, and pkg/b, which
// has none, with gremlins' report for pkg/a.
func mutationRepo(t *testing.T, report string) string {
	t.Helper()

	_, err := exec.LookPath("make")
	if err != nil {
		t.Skip("make is not installed here (the Debian Go image and CI's runner have it)")
	}

	dir := gittest.Repo(t, map[string]string{
		mutationPath: mutationConfig, makefile: gremlinsStub, ignoreFile: ignoreCache, "report.json": report,
	})
	gittest.Git(t, dir, "checkout", "-q", "-b", "work")
	commitAll(t, dir, map[string]string{"pkg/a/a.go": "package a\n", "pkg/b/b.go": "package b\n"})

	return dir
}

// The gate mutates the changed packages that have a floor, says which changed ones have none,
// and passes when each is at or over its floor.
func TestTheMutationGateHoldsTheChangedFlooredPackages(t *testing.T) {
	t.Parallel()

	dir := mutationRepo(t, aKilled)

	var out bytes.Buffer

	require.NoError(t, env{root: dir}.mutate(t.Context(), []string{baseFlag, onMain}, &out), out.String())
	assert.Contains(t, out.String(), "not mutated, no floor in gates/mutation.yaml: pkg/b\n")
	assert.Contains(t, out.String(), "pkg/a: efficacy 90.0% (floor 80%), 9 killed, 1 lived")
	assert.Contains(t, out.String(), "1 package(s) at or over their mutation floor")
}

// A package under its floor fails the gate, and the output says where gremlins' report is.
func TestAPackageUnderItsMutationFloorFailsTheGate(t *testing.T) {
	t.Parallel()

	dir := mutationRepo(t, aLived)

	var out bytes.Buffer

	err := env{root: dir}.mutate(t.Context(), []string{baseFlag, onMain}, &out)
	require.ErrorIs(t, err, errMutation, out.String())
	assert.Contains(t, err.Error(), "1 package(s)")
	assert.Contains(t, out.String(), "pkg/a: efficacy 50.0% (floor 80%)")
	assert.Contains(t, out.String(), "  gremlins' report: .cache/harness/mutation/pkg_a.json\n")
}

// A package whose mutants mostly timed out was not measured: the gate cannot run (exit 75,
// UNAVAILABLE), never a pass, and never a failure of its tests.
func TestAMutationRunThatTimedOutCannotRun(t *testing.T) {
	t.Parallel()

	dir := mutationRepo(t, aTimedOut)

	var out bytes.Buffer

	err := env{root: dir}.mutate(t.Context(), []string{baseFlag, onMain}, &out)
	require.ErrorIs(t, err, errCannotRun, out.String())
	require.NotErrorIs(t, err, errMutation)
	assert.Contains(t, out.String(), "30 timed out")
	assert.Contains(t, out.String(), "run it on a quieter machine")

	code, _ := exitFor(err)
	assert.Equal(t, runner.ExitUnavailable, code, "the runner reads it as UNAVAILABLE")
}

// The harness's exit codes: 75 for a command that cannot run here, which the runner reads as
// UNAVAILABLE; the other errors keep theirs.
func TestACommandThatCannotRunExitsUnavailable(t *testing.T) {
	t.Parallel()

	for err, want := range map[error]int{nil: 0, errFailed: 1, errIncomplete: exitIncomplete, usageError{"x"}: exitUsage, errCannotRun: runner.ExitUnavailable, errMutation: 1} {
		code, _ := exitFor(err)
		assert.Equal(t, want, code, "%v", err)
	}
}

// At night, --all mutates every floored package, whatever changed: here, nothing since work.
func TestTheNightMutatesEveryFlooredPackage(t *testing.T) {
	t.Parallel()

	dir := mutationRepo(t, aKilled)

	var out bytes.Buffer

	require.NoError(t, env{root: dir}.mutate(t.Context(), []string{"--all", baseFlag, "work"}, &out), out.String())
	assert.Contains(t, out.String(), "pkg/a: efficacy 90.0%")
	assert.NotContains(t, out.String(), "not mutated", "--all does not look at what changed")
	assert.Contains(t, out.String(), "1 package(s) at or over their mutation floor")
}

// When gremlins cannot run, the gate fails with gremlins' output, never a pass.
func TestAMutationRunThatFailsFailsTheGate(t *testing.T) {
	t.Parallel()

	dir := mutationRepo(t, aKilled)
	gittest.Write(t, dir, map[string]string{makefile: "go:\n\t@echo gremlins broke; exit 3\n"})

	var out bytes.Buffer

	err := env{root: dir}.mutate(t.Context(), []string{baseFlag, onMain}, &out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gremlins on pkg/a")
	assert.Contains(t, err.Error(), "gremlins broke")
	assert.NotContains(t, out.String(), "at or over their mutation floor")
}

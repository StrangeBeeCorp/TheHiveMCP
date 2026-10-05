package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/lint"
)

const (
	lintMakefile = "GO_IMAGE := golang:1.26.8-alpine\nGO_IMAGE_CGO := golang:1.26.8\n"
	xGo          = "internal/x/x.go"
)

// fakeTools stands in for docker: each command line is recorded, and the tool named by fail
// exits 1 with its output; the tool named by rewrite changes the file instead.
func fakeTools(t *testing.T, fail, rewrite string, file string) *[][]string {
	t.Helper()

	var calls [][]string

	prevExec, prevUp := toolExec, dockerRunning

	toolExec = func(_ context.Context, argv []string, _ io.Reader) (lint.Output, error) {
		calls = append(calls, argv)
		line := strings.Join(argv, " ")

		switch {
		case fail != "" && strings.Contains(line, fail):
			return lint.Output{Stdout: []byte("scripts/bad.sh:1: SC2086 (info): Double quote to prevent globbing.\n"), Code: 1}, nil
		case rewrite != "" && strings.Contains(line, rewrite):
			require.NoError(t, os.WriteFile(file, []byte("package x\n"), 0o600))
		}

		return lint.Output{Stderr: []byte("npm notice New major version of npm available!\n")}, nil
	}
	dockerRunning = func(context.Context) bool { return true }

	t.Cleanup(func() { toolExec, dockerRunning = prevExec, prevUp })

	return &calls
}

func lintRepo(t *testing.T) string {
	t.Helper()

	dir := gittest.Repo(t, map[string]string{
		registryPath: editRegistry, limitsPath: editLimits, gitignore: ignoreCache, "Makefile": lintMakefile,
		xGo: pkgX,
	})
	t.Chdir(dir)

	return dir
}

// bin/harness lint --staged reads what the commit carries: a shellcheck finding fails it, with
// the finding and a last line that names the check, as the gate's summary.
//
//nolint:paralleltest // t.Chdir, and the tools are replaced for the test
func TestLintFailsOnAFindingAndNamesIt(t *testing.T) {
	dir := lintRepo(t)
	gittest.Write(t, dir, map[string]string{"scripts/bad.sh": "echo $1\n", "scripts/unstaged.sh": "echo $2\n"})
	gittest.Git(t, dir, "add", "scripts/bad.sh")

	calls := fakeTools(t, "shellcheck", "", "")

	var out strings.Builder

	err := dispatch(t.Context(), []string{cmdLint, "--staged"}, strings.NewReader(""), &out)
	require.ErrorIs(t, err, errFailed)
	assert.Contains(t, out.String(), "=== shellcheck ===\nscripts/bad.sh:1: SC2086")
	assert.True(t, strings.HasSuffix(out.String(), "✗ lint failed (1 sh): shellcheck\n"), out.String())

	for _, c := range *calls {
		assert.NotContains(t, strings.Join(c, " "), "unstaged.sh", "only what the commit carries")
	}
}

// bin/harness fmt rewrites by default and lists what it changed; with --check it only reports.
//
//nolint:paralleltest // t.Chdir, and the tools are replaced for the test
func TestFmtRewritesAndSaysWhat(t *testing.T) {
	dir := lintRepo(t)
	gofile := filepath.Join(dir, filepath.FromSlash(xGo))
	require.NoError(t, os.WriteFile(gofile, []byte("package  x\n"), 0o600))

	fakeTools(t, "", "gofmt -l -w", gofile)

	var out strings.Builder

	require.NoError(t, dispatch(t.Context(), []string{cmdFmt, "--changed"}, strings.NewReader(""), &out))
	assert.Equal(t, "Formatted:\n  internal/x/x.go\n✓ formatted 1 of 1 go\n", out.String())

	out.Reset()
	require.NoError(t, dispatch(t.Context(), []string{cmdFmt}, strings.NewReader(""), &out))
	assert.Equal(t, "✓ already formatted (1 go)\n", out.String(), "nothing left to rewrite, and only the kinds fmt formats counted")

	fakeTools(t, "", "", "")
	out.Reset()
	require.NoError(t, dispatch(t.Context(), []string{cmdFmt, "--all", "--check"}, strings.NewReader(""), &out))
	assert.Equal(t, "✓ fmt check passed (1 go)\n", out.String())
}

//nolint:paralleltest // t.Chdir, and the tools are replaced for the test
func TestLintNeedsDockerAndKnownArguments(t *testing.T) {
	lintRepo(t)
	fakeTools(t, "", "", "")

	var usage usageError
	require.ErrorAs(t, dispatch(t.Context(), []string{cmdLint, "--hook"}, strings.NewReader(""), io.Discard), &usage)

	dockerRunning = func(context.Context) bool { return false }

	require.ErrorContains(t, dispatch(t.Context(), []string{cmdFmt}, strings.NewReader(""), io.Discard), "docker is not running")
}

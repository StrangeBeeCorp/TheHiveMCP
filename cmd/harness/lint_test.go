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
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/proc"
)

const (
	lintMakefile = "GO_IMAGE := golang:1.26.8-alpine\nGO_IMAGE_CGO := golang:1.26.8\n"
	yGo          = "internal/y/y.go" // and xGo, hook_test.go's
	npmNotice    = "npm notice New major version of npm available!\n"
)

// fakeTools stands in for docker: every command line is recorded and answered by respond, given
// the line; nil answers a clean run, with an npm notice on stderr as noise.
func fakeTools(t *testing.T, respond func(line string) *proc.Output) *[][]string {
	t.Helper()

	var calls [][]string

	prevExec, prevUp := toolExec, dockerRunning

	toolExec = func(_ context.Context, argv []string, _ io.Reader) (proc.Output, error) {
		calls = append(calls, argv)

		if respond != nil {
			if out := respond(strings.Join(argv, " ")); out != nil {
				return *out, nil
			}
		}

		return proc.Output{Stderr: []byte(npmNotice)}, nil
	}
	dockerRunning = func(context.Context) bool { return true }

	t.Cleanup(func() { toolExec, dockerRunning = prevExec, prevUp })

	return &calls
}

// rewrites answers the line naming tool by writing content to file.
func rewrites(t *testing.T, tool, file, content string) func(string) *proc.Output {
	t.Helper()

	return func(line string) *proc.Output {
		if strings.Contains(line, tool) {
			require.NoError(t, os.WriteFile(file, []byte(content), 0o600))
		}

		return nil
	}
}

func lintRepo(t *testing.T) string {
	t.Helper()

	dir := gittest.Repo(t, map[string]string{
		registryPath: editRegistry, limitsPath: editLimits, gitignore: ignoreCache, "Makefile": lintMakefile,
		xGo: pkgX, yGo: "package y\n",
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

	calls := fakeTools(t, func(line string) *proc.Output {
		if strings.Contains(line, "shellcheck") {
			return &proc.Output{Stdout: []byte("scripts/bad.sh:1: SC2086 (info): Double quote to prevent globbing.\n"), Code: 1}
		}

		return nil
	})

	out, err := harness(t, cmdLint, "--staged")
	require.ErrorIs(t, err, errFailed)
	assert.Contains(t, out, "=== shellcheck ===\nscripts/bad.sh:1: SC2086")
	assert.True(t, strings.HasSuffix(out, "✗ lint failed (1 sh): shellcheck\n"), out)

	for _, c := range *calls {
		assert.NotContains(t, strings.Join(c, " "), "unstaged.sh", "only what the commit carries")
	}
}

// lint --fix reports what the formatters rewrote and, apart, what golangci-lint's safe fixes
// changed: those rewrite the whole module, so a Go file outside the scope is reported too.
//
//nolint:paralleltest // t.Chdir, and the tools are replaced for the test
func TestLintFixReportsEveryFileItChanged(t *testing.T) {
	dir := lintRepo(t)
	gittest.Write(t, dir, map[string]string{xGo: "package  x\n"})

	formatX, fixY := rewrites(t, "gofmt -l -w", filepath.Join(dir, xGo), pkgX), rewrites(t, "run --fix", filepath.Join(dir, yGo), "package y // fixed\n")
	fakeTools(t, func(line string) *proc.Output {
		formatX(line)
		fixY(line)

		return nil
	})

	out, err := harness(t, cmdLint, "--changed", "--fix")
	require.NoError(t, err)
	assert.Equal(t, "Auto-fixed:\n  "+xGo+"\nAuto-fixed (lint):\n  "+yGo+"\n✓ lint passed (1 go)\n", out, "y.go is out of the scope")
}

// bin/harness fmt rewrites by default and lists what it changed; with --check, the fmt gate, a
// file a formatter lists fails it.
//
//nolint:paralleltest // t.Chdir, and the tools are replaced for the test
func TestFmtRewritesOrFailsTheCheck(t *testing.T) {
	dir := lintRepo(t)
	gittest.Write(t, dir, map[string]string{xGo: "package  x\n"})

	fakeTools(t, rewrites(t, "gofmt -l -w", filepath.Join(dir, xGo), pkgX))

	out, err := harness(t, cmdFmt, "--changed")
	require.NoError(t, err)
	assert.Equal(t, "Formatted:\n  "+xGo+"\n✓ formatted 1 of 1 go\n", out)

	out, err = harness(t, cmdFmt)
	require.NoError(t, err)
	assert.Equal(t, "✓ already formatted (2 go)\n", out, "nothing left to rewrite, and only the kinds fmt formats counted")

	fakeTools(t, nil)

	out, err = harness(t, cmdFmt, "--all", "--check")
	require.NoError(t, err)
	assert.Equal(t, "✓ fmt check passed (2 go)\n", out, "npm's notice on stderr lists no file")

	fakeTools(t, func(line string) *proc.Output {
		if strings.Contains(line, "gofmt -l /app/") {
			return &proc.Output{Stdout: []byte("/app/" + yGo + "\n"), Stderr: []byte(npmNotice)}
		}

		return nil
	})

	out, err = harness(t, cmdFmt, "--all", "--check")
	require.ErrorIs(t, err, errFailed)
	assert.Equal(t, "=== gofmt (would reformat) ===\n"+yGo+"\n\n✗ fmt: files need formatting (run make fmt): gofmt (would reformat)\n", out)
}

// A formatter that could not run fails make fmt instead of reading as "already formatted".
//
//nolint:paralleltest // t.Chdir, and the tools are replaced for the test
func TestFmtFailsWhenAFormatterCannotRun(t *testing.T) {
	lintRepo(t)
	fakeTools(t, func(line string) *proc.Output {
		if strings.Contains(line, "golangci-lint fmt") {
			return &proc.Output{Stderr: []byte("Unable to find image locally\n"), Code: 125}
		}

		return nil
	})

	out, err := harness(t, cmdFmt)
	require.ErrorIs(t, err, errFailed)
	assert.Contains(t, out, "=== golangci-lint fmt ===\nUnable to find image locally\n")
	assert.True(t, strings.HasSuffix(out, "✗ fmt: a formatter could not run: golangci-lint fmt\n"), out)
}

// lint and fmt read the repository alone: a registry or limits file being edited does not stop
// them, as it did not stop the scripts they replace.
//
//nolint:paralleltest // t.Chdir, and the tools are replaced for the test
func TestLintAndFmtNeedNoRegistry(t *testing.T) {
	dir := lintRepo(t)
	gittest.Write(t, dir, map[string]string{registryPath: "gates: [half edited\n", limitsPath: "structure: {max_lines: -\n"})
	gittest.Git(t, dir, "add", xGo)
	fakeTools(t, nil)

	_, err := harness(t, cmdLint, "--staged")
	require.NoError(t, err)

	_, err = harness(t, cmdFmt, "--check")
	require.NoError(t, err)
}

//nolint:paralleltest // t.Chdir, and the tools are replaced for the test
func TestLintNeedsDockerKnownArgumentsAndRealPaths(t *testing.T) {
	lintRepo(t)
	fakeTools(t, nil)

	var usage usageError

	_, err := harness(t, cmdLint, "--hook")
	require.ErrorAs(t, err, &usage)

	_, err = harness(t, cmdLint, "Makefile", "scripts/harnes.mk", "internal")
	require.ErrorContains(t, err, "not a file of the checkout: internal, scripts/harnes.mk", "a typo, and a directory")

	dockerRunning = func(context.Context) bool { return false }

	_, err = harness(t, cmdFmt)
	require.ErrorContains(t, err, "docker is not running")
}

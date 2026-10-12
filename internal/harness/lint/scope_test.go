package lint_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/lint"
)

const (
	flagFix     = "--fix"
	flagChanged = "--changed"
	newMD       = "new.md"
	head        = "HEAD"
	committedMD = "committed.md"
	stagedYAML  = "staged.yaml"
)

func TestParseArgsReadsTheScopeAndTheMode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
		fix  bool // the command's default
		want lint.Options
	}{
		{"lint checks every file by default", nil, false, lint.Options{}},
		{"fmt fixes by default", nil, true, lint.Options{Fix: true}},
		{"the stop's scope", []string{flagChanged, head, "--check"}, true, lint.Options{Scope: lint.Scope{Changed: true, Commit: head}}},
		{"a flag after --changed is no commit", []string{flagChanged, flagFix}, false, lint.Options{Scope: lint.Scope{Changed: true}, Fix: true}},
		{"the commit's scope", []string{"--staged"}, false, lint.Options{Scope: lint.Scope{Staged: true}}},
		{"the last scope wins", []string{"--staged", "--all"}, false, lint.Options{}},
		{"paths", []string{makefile, "./scripts/../scripts/harness.mk"}, false, lint.Options{Scope: lint.Scope{Paths: []string{makefile, mkFile}}}},
	}

	for _, c := range cases {
		got, err := lint.ParseArgs(c.args, c.fix)
		require.NoError(t, err, c.name)
		assert.Equal(t, c.want, got, c.name)
	}

	for _, bad := range [][]string{{"--hook"}, {"../elsewhere.go"}, {"/etc/passwd"}} {
		_, err := lint.ParseArgs(bad, false)
		require.ErrorIs(t, err, lint.ErrUsage, "%v", bad)
	}
}

// Each git scope lists its files that still exist: a deleted file reaches no tool.
func TestSelectListsTheScopesFilesThatExist(t *testing.T) {
	t.Parallel()

	dir := gittest.Repo(t, map[string]string{goFile: "package x\n", shFile: "#!/bin/sh\n", ".gitignore": ".cache/\n"})
	gittest.Write(t, dir, map[string]string{committedMD: "# a\n"})
	gittest.Git(t, dir, "add", committedMD)
	gittest.Commit(t, dir)

	gittest.Write(t, dir, map[string]string{goFile: "package x // edited\n", newMD: "# new\n", stagedYAML: "a: 1\n", ".cache/x.go": "package x\n"})
	gittest.Git(t, dir, "add", stagedYAML)
	gittest.Git(t, dir, "rm", "-q", shFile)

	selected := func(s lint.Scope) []string {
		t.Helper()

		paths, err := s.Select(t.Context(), dir)
		require.NoError(t, err)

		return paths
	}

	assert.Equal(t, []string{".gitignore", committedMD, goFile, newMD, stagedYAML}, selected(lint.Scope{}),
		"every file, untracked ones too, ignored ones not")
	assert.Equal(t, []string{stagedYAML}, selected(lint.Scope{Staged: true}), "the staged deletion reaches no tool")
	assert.Equal(t, []string{goFile, newMD, stagedYAML}, selected(lint.Scope{Changed: true}))
	assert.Equal(t, []string{committedMD, goFile, newMD, stagedYAML}, selected(lint.Scope{Changed: true, Commit: head}),
		"the last commit's files join, as they do at an agent's stop")
	assert.Equal(t, []string{goFile, newMD}, selected(lint.Scope{Paths: []string{newMD, goFile}}))

	_, err := lint.Scope{Paths: []string{goFile, "missing.go", "internal"}}.Select(t.Context(), dir)
	require.ErrorContains(t, err, "not a file of the checkout: internal, missing.go", "a path named is no git scope: a typo or a directory is said")

	_, err = lint.Scope{Changed: true, Commit: "no-such-commit"}.Select(t.Context(), dir)
	require.ErrorContains(t, err, `"no-such-commit" is not a commit`)
}

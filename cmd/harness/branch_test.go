package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"
)

// onMain is the branch a person's merge moves.
const onMain = "main"

// withRemotes gives the checkout an origin and a public mirror, both bare, and returns them.
func withRemotes(t *testing.T, dir string) (string, string) {
	t.Helper()

	origin, public := t.TempDir(), t.TempDir()
	gittest.Git(t, origin, "init", "-q", "--bare")
	gittest.Git(t, public, "init", "-q", "--bare")
	gittest.Git(t, dir, "remote", "add", "origin", origin)
	gittest.Git(t, dir, "remote", "add", "public", public)

	return origin, public
}

// make push never pushes main, a release branch or a detached HEAD.
//
//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestPushRefusesMainAReleaseBranchAndADetachedHEAD(t *testing.T) {
	dir := checkout(t)
	origin, _ := withRemotes(t, dir)

	for _, c := range []struct{ checkout, refusal string }{
		{onMain, "main changes only when a person merges a pull request"},
		{"release/1.2", "release/1.2 changes only when a person merges a pull request"},
		{"--detach", "HEAD is detached"},
	} {
		if c.checkout == "release/1.2" {
			gittest.Git(t, dir, "checkout", "-q", "-b", c.checkout)
		} else {
			gittest.Git(t, dir, "checkout", "-q", c.checkout)
		}

		_, err := harness(t, "push")

		var usage usageError
		require.ErrorAs(t, err, &usage, c.checkout)
		assert.Contains(t, usage.msg, c.refusal)
	}

	assert.Empty(t, gittest.Git(t, origin, "for-each-ref"), "nothing reached origin")
}

// make push sends the branch checked out to origin under its own name, and nowhere else.
//
//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestPushSendsTheBranchToOriginUnderItsName(t *testing.T) {
	dir := checkout(t)
	origin, public := withRemotes(t, dir)
	head := strings.TrimSpace(gittest.Git(t, dir, "rev-parse", "HEAD"))

	_, err := harness(t, "push")
	require.NoError(t, err)
	assert.Equal(t, head, strings.TrimSpace(gittest.Git(t, origin, "rev-parse", "refs/heads/work")))
	assert.Equal(t, "origin", strings.TrimSpace(gittest.Git(t, dir, "config", "branch.work.remote")), "-u: the branch tracks origin")

	gittest.Git(t, dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false",
		"commit", "-q", "--amend", "--allow-empty", "-m", "rewritten")

	_, err = harness(t, "push")
	require.Error(t, err, "a rewritten branch needs --force")

	_, err = harness(t, "push", "--force")
	require.NoError(t, err)
	assert.Equal(t, strings.TrimSpace(gittest.Git(t, dir, "rev-parse", "HEAD")), strings.TrimSpace(gittest.Git(t, origin, "rev-parse", "refs/heads/work")))
	assert.Empty(t, gittest.Git(t, public, "for-each-ref"), "the public mirror is a person's")
}

// The branch check, at the commit cadence, refuses a commit on main and passes one on a branch.
//
//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestTheBranchCheckRefusesACommitOnMain(t *testing.T) {
	dir := checkout(t)

	out, err := harness(t, "check", "branch")
	require.NoError(t, err)
	assert.Equal(t, "branch: clean\n", out)

	gittest.Git(t, dir, "checkout", "-q", onMain)

	out, err = harness(t, "check", "branch")
	require.ErrorIs(t, err, errFailed)
	assert.Contains(t, out, "this commit is on main")
}

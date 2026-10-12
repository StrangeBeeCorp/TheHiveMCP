package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"
)

// The repository's pre-push hook lets make pr's notes go to origin and refuses them to any other
// remote, the public mirror above all, while a branch still goes there.
func TestTheTimingNotesGoToOriginOnly(t *testing.T) {
	t.Parallel()

	hooks, err := filepath.Abs(filepath.Join("..", "..", ".githooks"))
	require.NoError(t, err)

	dir := gittest.Repo(t, nil)
	gittest.Git(t, dir, "config", "core.hooksPath", hooks)

	for _, name := range []string{"origin", "public"} {
		bare := filepath.Join(t.TempDir(), name+".git")
		gittest.Git(t, dir, "init", "-q", "--bare", bare)
		gittest.Git(t, dir, "remote", "add", name, bare)
	}

	const ref = "refs/notes/harness/0123456789abcdef"

	require.NoError(t, gitx.AddNote(t.Context(), dir, ref, gitx.Head(t.Context(), dir), []byte("{}")))

	push := func(args ...string) (string, error) {
		cmd := exec.CommandContext(t.Context(), "git", append([]string{gitPush, "-q"}, args...)...) // #nosec G204 -- the test's own push
		cmd.Dir = dir

		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		out, err := cmd.CombinedOutput()

		return string(out), err
	}

	out, err := push("origin", ref+":"+ref)
	require.NoError(t, err, out)

	out, err = push("public", ref+":"+ref)
	require.Error(t, err, "the public mirror refuses the notes")
	assert.Contains(t, out, "goes to origin only, never to public")

	out, err = push("--mirror", "public")
	require.Error(t, err, "a mirror push takes the notes along: refused whole")
	assert.Contains(t, out, "never to public")

	out, err = push("public", "main")
	require.NoError(t, err, "a branch still goes to the mirror: %s", out)
}

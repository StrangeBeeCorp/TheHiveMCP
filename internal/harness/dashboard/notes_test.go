package dashboard

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"
)

const (
	noteA = `{"schema":1,"head":"a","branch":"b","verdict":"PASS","started":"2026-10-01T10:00:00Z","seconds":600,` +
		`"machine":{"os":"darwin","arch":"arm64","cpus":15,"memory_gb":24,"ci":false},"gates":[{"gate":"integration","verdict":"PASS","seconds":345}]}`
	noteB = `{"schema":1,"head":"b","branch":"b","verdict":"FAIL","started":"2026-10-02T10:00:00Z","seconds":60,` +
		`"machine":{"os":"linux","arch":"amd64","cpus":4,"ci":true},"gates":[]}`
)

// The page reads every clone's notes from origin and this clone's own, which the fetch must not
// overwrite, each run once; a note of two merged runs gives both, and a line of another schema
// is skipped.
func TestTheNotesOfEveryCloneAreReadEachRunOnce(t *testing.T) {
	t.Parallel()

	other := gittest.Repo(t, nil)
	head := gitx.Head(t.Context(), other)
	require.NoError(t, gitx.AddNote(t.Context(), other, "refs/notes/harness/other", head, []byte(noteA+"\n"+noteB+"\n{\"schema\":2}")))

	remote := filepath.Join(t.TempDir(), "origin.git")
	gittest.Git(t, other, "clone", "-q", "--bare", other, remote)
	gittest.Git(t, other, "--git-dir="+remote, "fetch", "-q", other, "refs/notes/harness/other:refs/notes/harness/other")

	mine := filepath.Join(t.TempDir(), "mine")
	gittest.Git(t, other, "clone", "-q", remote, mine)
	require.NoError(t, gitx.AddNote(t.Context(), mine, "refs/notes/harness/mine", head, []byte(noteA)))

	runs, err := readNotes(t.Context(), mine)
	require.NoError(t, err)
	require.Len(t, runs, 2, "noteA sits under both refs: one run")

	labels := []string{runs[0].MachineLabel, runs[1].MachineLabel}
	assert.ElementsMatch(t, []string{"darwin/arm64, 15 CPUs, 24 GB", "CI linux/amd64, 4 CPUs"}, labels)
	assert.Equal(t, "refs/notes/harness/mine", gittest.Git(t, mine, "for-each-ref", "--format=%(refname)", "refs/notes/harness"),
		"origin's notes went beside this clone's, not over them")

	alone := gittest.Repo(t, nil)
	require.NoError(t, gitx.AddNote(t.Context(), alone, "refs/notes/harness/x", gitx.Head(t.Context(), alone), []byte(noteB)))

	runs, err = readNotes(t.Context(), alone)
	require.Error(t, err, "no origin to fetch from: said")
	assert.Contains(t, err.Error(), "only this clone's are shown")
	assert.Len(t, runs, 1, "and this clone's notes are still read")
}

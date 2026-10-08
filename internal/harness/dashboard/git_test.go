package dashboard

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"
)

const opus55 = "Claude Opus 5.5"

// One model has one name, however a commit wrote it: the trailer's ID, Co-Authored-By's name with
// its context window, or a dated ID.
func TestAModelHasOneNameHoweverACommitWroteIt(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		"claude-opus-5-5":              opus55,
		opus55:                         opus55,
		"Claude Opus 4.8 (1M context)": "Claude Opus 4.8",
		"claude-haiku-4-5-20251001":    "Claude Haiku 4.5",
		"claude-fable-5-1":             "Claude Fable 5.1",
		"Claude Fable 5":               "Claude Fable 5",
		"":                             "",
	} {
		assert.Equal(t, want, modelName(in), in)
	}
}

// An agent's trailers name its model, effort and the repository's skills, not the count of the
// user's own; an older commit has only Co-Authored-By; a person's commit has neither.
func TestACommitsTrailersSayWhichAgentWroteIt(t *testing.T) {
	t.Parallel()

	var c Commit

	trailers(&c, "[DL-1] feat: x\n\nWhy.\n\nCo-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>\n"+
		"Agent-Model: claude-opus-5-5\nAgent-Effort: high\nSkills-Used: develop-feature, review (+7 from the user's plugins)\n")
	assert.Equal(t, Commit{Model: opus55, Effort: "high", Skills: []string{"develop-feature", "review"}}, c)

	var old Commit

	trailers(&old, "fix: y\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>\n")
	assert.Equal(t, Commit{Model: "Claude Opus 4.8"}, old)

	var none Commit

	trailers(&none, "fix: z\n\nSkills-Used: none\n")
	assert.Equal(t, Commit{}, none, "a person's commit; no skill is none")
}

// The history is read from a real repository: its commits with their trailers, the commits of
// main's first-parent history, and the release tags.
func TestTheHistoryIsReadFromGit(t *testing.T) {
	t.Parallel()

	dir := gittest.Repo(t, map[string]string{"a.txt": "a\n"})
	gittest.Git(t, dir, "-c", "user.name=An Agent", "-c", "user.email=agent@example.com", "-c", "commit.gpgsign=false",
		"-c", "core.hooksPath=/dev/null", "commit", "-q", "--allow-empty", "-m", "feat: b\n\nAgent-Model: claude-opus-5-5\nAgent-Effort: low")
	gittest.Git(t, dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "tag.gpgsign=false", "tag", "-a", "v1.0.0", "-m", "v1.0.0")
	gittest.Git(t, dir, "remote", "add", "origin", "git@github.com:Owner/Repo.git")

	h, err := readGit(t.Context(), Options{Root: dir, Main: mainBranch, Since: time.Now().AddDate(0, 0, -1)})
	require.NoError(t, err)

	assert.Equal(t, "Owner/Repo", h.Repo)
	assert.Len(t, h.Head, 7)
	require.Len(t, h.Commits, 2)
	assert.Equal(t, "An Agent", h.Commits[0].Author, "the newest first")
	assert.Equal(t, opus55, h.Commits[0].Model)
	assert.Equal(t, "low", h.Commits[0].Effort)
	assert.Empty(t, h.Commits[1].Model, "the first commit is a person's")
	assert.Len(t, h.Merges, 2)
	require.Len(t, h.Tags, 1)
	assert.Equal(t, "v1.0.0", h.Tags[0].Name)

	_, err = readGit(t.Context(), Options{Root: t.TempDir(), Main: mainBranch})
	require.ErrorIs(t, err, errNoGit)
}

func TestARemoteNamesItsRepository(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		"https://github.com/StrangeBee/TheHiveMCP.git": "StrangeBee/TheHiveMCP",
		"git@github.com:StrangeBee/TheHiveMCP.git":     "StrangeBee/TheHiveMCP",
		"git@github-perso:Owner/Repo.git":              "Owner/Repo",
		"/srv/repo.git":                                "/srv/repo",
	} {
		assert.Equal(t, want, repoName(in), in)
	}
}

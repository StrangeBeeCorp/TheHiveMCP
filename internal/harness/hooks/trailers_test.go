package hooks_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/hooks"
)

const (
	session    = "11111111-2222-3333-4444-555555555555"
	claudeCode = "CLAUDECODE"
	sessionVar = "CLAUDE_CODE_SESSION_ID"
	effortVar  = "CLAUDE_EFFORT"
)

// transcript writes a session's transcript under home's Claude Code projects, in project.
func transcript(t *testing.T, home, project, content string, age time.Duration) {
	t.Helper()

	p := filepath.Join(home, ".claude", "projects", project, session+".jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	require.NoError(t, os.Chtimes(p, time.Now().Add(-age), time.Now().Add(-age)))
}

// env is a lookup over a map, as os.Getenv is over the process.
func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

// checkoutWithSkill is a checkout that defines one skill of its own, develop-feature.
func checkoutWithSkill(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	writeFile(t, root, ".claude/skills/develop-feature/SKILL.md", "# develop-feature\n")

	return root
}

// A commit made from a Claude Code session carries its model (the latest real one of the
// transcript), its reasoning effort (CLAUDE_EFFORT), and the skills it used: the repository's own by
// name, sorted, once each, the user's plugins' only counted, for the public mirror. A session's newest
// transcript is read when it has several (one per working directory), whatever the length of its lines.
func TestAnAgentsCommitCarriesItsModelEffortAndSkills(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	transcript(t, home, "-old-dir", `{"message":{"model":"claude-old"}}`+"\n", time.Hour)
	transcript(t, home, "-repo", `{"message":{"role":"user","content":"go"}}
{"message":{"model":"claude-sonnet-5-5"},"attributionSkill":"sb-personal:review"}
{"message":{"content":"`+strings.Repeat("x", 20<<20)+`"}}
{"message":{"model":"claude-opus-5-5"},"attributionSkill":"develop-feature"}
{"message":{"model":"claude-opus-5-5"},"attributionSkill":"sb-personal:review"}
not json
{"message":{"model":"<synthetic>"}}
`, 0)

	got := hooks.Trailers(env(map[string]string{claudeCode: "1", sessionVar: session, effortVar: "xhigh"}), home, checkoutWithSkill(t))
	assert.Equal(t, []string{
		"Agent-Model: claude-opus-5-5",
		"Agent-Effort: xhigh",
		"Skills-Used: develop-feature (+1 from the user's plugins)",
	}, got)
}

// What the session cannot tell is written unknown, never guessed: no effort in the environment,
// no transcript, no skill.
func TestWhatASessionCannotTellIsUnknown(t *testing.T) {
	t.Parallel()

	got := hooks.Trailers(env(map[string]string{claudeCode: "1", sessionVar: session}), t.TempDir(), t.TempDir())
	assert.Equal(t, []string{"Agent-Model: unknown", "Agent-Effort: unknown", "Skills-Used: none"}, got)
}

// A transcript that cannot be read to its end tells nothing for sure: the model and the skills are
// unknown, not what was read before the failure.
func TestATranscriptReadInPartIsUnknown(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	// A directory where the transcript should be: it opens, and reading it fails.
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".claude", "projects", "-repo", session+".jsonl"), 0o750))

	got := hooks.Trailers(env(map[string]string{claudeCode: "1", sessionVar: session, effortVar: "high"}), home, t.TempDir())
	assert.Equal(t, []string{"Agent-Model: unknown", "Agent-Effort: high", "Skills-Used: unknown"}, got)
}

// A person's commit, outside Claude Code, carries none.
func TestAPersonsCommitCarriesNoTrailer(t *testing.T) {
	t.Parallel()

	assert.Empty(t, hooks.Trailers(env(map[string]string{effortVar: "high"}), t.TempDir(), t.TempDir()))
	assert.Empty(t, hooks.Trailers(env(map[string]string{claudeCode: "0"}), t.TempDir(), t.TempDir()))
}

package hooks_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/hooks"
)

const (
	session    = "11111111-2222-3333-4444-555555555555"
	claudeCode = "CLAUDECODE"
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

// A commit made from a Claude Code session carries its model (the latest of the transcript), its
// reasoning effort (CLAUDE_EFFORT), and the skills it used (sorted, once each); a session's newest
// transcript is read when it has several (one per working directory).
func TestAnAgentsCommitCarriesItsModelEffortAndSkills(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	transcript(t, home, "-old-dir", `{"message":{"model":"claude-old"}}`+"\n", time.Hour)
	transcript(t, home, "-repo", `{"message":{"role":"user","content":"go"}}
{"message":{"model":"claude-opus-5-5"},"attributionSkill":"sb-personal:review"}
{"message":{"model":"<synthetic>"}}
{"message":{"model":"claude-opus-5-5"},"attributionSkill":"develop-feature"}
{"message":{"model":"claude-opus-5-5"},"attributionSkill":"sb-personal:review"}
not json
`, 0)

	got := hooks.Trailers(env(map[string]string{claudeCode: "1", "CLAUDE_CODE_SESSION_ID": session, "CLAUDE_EFFORT": "xhigh"}), home)
	assert.Equal(t, []string{
		"Agent-Model: claude-opus-5-5",
		"Agent-Effort: xhigh",
		"Skills-Used: develop-feature, sb-personal:review",
	}, got)
}

// What the session cannot tell is written unknown, never guessed: no effort in the environment,
// no transcript, no skill.
func TestWhatASessionCannotTellIsUnknown(t *testing.T) {
	t.Parallel()

	got := hooks.Trailers(env(map[string]string{claudeCode: "1", "CLAUDE_CODE_SESSION_ID": session}), t.TempDir())
	assert.Equal(t, []string{"Agent-Model: unknown", "Agent-Effort: unknown", "Skills-Used: none"}, got)
}

// A person's commit, outside Claude Code, carries none.
func TestAPersonsCommitCarriesNoTrailer(t *testing.T) {
	t.Parallel()

	assert.Empty(t, hooks.Trailers(env(map[string]string{"CLAUDE_EFFORT": "high"}), t.TempDir()))
	assert.Empty(t, hooks.Trailers(env(map[string]string{claudeCode: "0"}), t.TempDir()))
}

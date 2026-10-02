package hooks_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/hooks"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
)

// sessionContext is what SessionStart tells the agent.
func sessionContext(t *testing.T, root, source string) string {
	t.Helper()

	v := decode(t, hooks.SessionStart(context.Background(), root, hooks.Input{Source: source}))
	spec, ok := v["hookSpecificOutput"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "SessionStart", spec["hookEventName"])

	text, _ := spec["additionalContext"].(string)

	return text
}

// SessionStart tells the agent where it is and what the last check-fast said; after a compaction
// it gives back what PreCompact saved, the red report and a BLOCKED report among it.
func TestSessionStartGivesBackWhereTheWorkStands(t *testing.T) {
	t.Parallel()

	root := gittest.Repo(t, map[string]string{"internal/a.go": "package a\n"})
	gittest.Git(t, root, "checkout", "-q", "-b", "feat/DL-1234-x")

	start := sessionContext(t, root, "startup")
	assert.Contains(t, start, "branch feat/DL-1234-x")
	assert.Contains(t, start, "ticket DL-1234")
	assert.Contains(t, start, "the main checkout")
	assert.Contains(t, start, "No check-fast has run")

	check := &fakeCheck{result: hooks.Check{Red: true, Report: "unit: FAIL TestA"}}
	s := hooks.Stopper{Root: root, MaxRounds: 6, Check: check.run, Fingerprint: same}
	_ = s.Stop(context.Background(), hooks.Input{SessionID: "s", AgentType: coderType})

	gittest.Write(t, root, map[string]string{"internal/a.go": "package a // edited\n", policy.BlockedFile: "tried x; need y\n"})
	assert.Empty(t, hooks.PreCompact(context.Background(), root).Stdout)

	after := sessionContext(t, root, "compact")
	assert.Contains(t, after, "was red:\nunit: FAIL TestA")
	assert.Contains(t, after, "This task is BLOCKED")
	assert.Contains(t, after, "tried x; need y")
	assert.Contains(t, after, "Saved before the compaction:")
	assert.Contains(t, after, "internal/a.go", "the changed file")
	assert.Contains(t, after, "Red at the last check-fast:\nunit: FAIL TestA")

	assert.NotContains(t, sessionContext(t, root, "startup"), "Saved before the compaction", "only after a compaction")
}

// A static run is reported as one: the Tester's green is not the code's.
func TestSessionStartSaysAStaticRunWasStatic(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	s := hooks.Stopper{Root: root, MaxRounds: 6, Check: (&fakeCheck{}).run, Fingerprint: same}
	_ = s.Stop(context.Background(), hooks.Input{SessionID: "s", AgentType: testerType})

	assert.Contains(t, sessionContext(t, root, "startup"), "ran the static gates only")

	incomplete := hooks.Stopper{Root: root, MaxRounds: 6, Check: (&fakeCheck{result: hooks.Check{Incomplete: true, Report: "lint: no Docker"}}).run, Fingerprint: same}
	_ = incomplete.Stop(context.Background(), hooks.Input{SessionID: "s", AgentType: coderType})

	assert.Contains(t, sessionContext(t, root, "startup"), "could not run every gate")
	require.NoError(t, os.RemoveAll(filepath.Join(root, ".cache")))
	assert.Contains(t, sessionContext(t, root, "startup"), "No check-fast has run")
}

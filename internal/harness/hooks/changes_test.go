package hooks_test

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/hooks"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
)

// exploreType is a read-only agent type.
const exploreType = "Explore"

// checkout is a fake one: its listing is what the test says it is now.
type checkout struct{ now hooks.Snapshot }

func (c *checkout) list(context.Context) (hooks.Snapshot, error) {
	return hooks.Snapshot{Head: c.now.Head, Files: maps.Clone(c.now.Files), Dirty: slices.Clone(c.now.Dirty)}, nil
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()

	p := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
}

func readFile(t *testing.T, p string) string {
	t.Helper()

	data, err := os.ReadFile(p) // #nosec G304 -- a file the test wrote
	require.NoError(t, err)

	return string(data)
}

func (c *checkout) set(path, sum string) { c.now.Files[path] = sum }

func started() *checkout {
	return &checkout{now: hooks.Snapshot{Head: "aaaaaaaaaa", Files: map[string]string{
		"internal/tools/search/handler.go":      "1",
		"internal/tools/search/search_test.go":  "2",
		"CLAUDE.md":                             "3",
		"baselines/surface/tokens.json":         "4",
		"internal/tools/search/testdata/a.json": "5",
	}}}
}

// What an agent changed is judged by its role, however it was changed: the Coder's test, the
// Tester's code, any agent's protected file, a reviewer's anything, and a commit by a role that
// does not commit. Baselines are changed by their make targets, and the main session is free.
func TestAnAgentIsHeldToWhatItsRoleMayChange(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		agentType string
		change    func(c *checkout)
		want      []string
	}{
		{coderType, func(c *checkout) { c.set("internal/tools/search/search_test.go", "x") }, []string{"search_test.go is a test"}},
		{coderType, func(c *checkout) { delete(c.now.Files, "internal/tools/search/testdata/a.json") }, []string{"testdata/a.json is a test"}},
		{coderType, func(c *checkout) { c.set("internal/tools/search/handler.go", "x") }, nil},
		{coderType, func(c *checkout) { c.now.Head = "bbbbbbbbbb" }, []string{"keep its changes: git reset --soft aaaaaaaaaa"}},
		{testerType, func(c *checkout) { c.set("internal/tools/search/new.go", "x") }, []string{"new.go is code"}},
		{testerType, func(c *checkout) { c.set("internal/tools/search/search_test.go", "x") }, nil},
		{"general-purpose", func(c *checkout) { c.set("CLAUDE.md", "x") }, []string{"CLAUDE.md is harness policy"}},
		{"general-purpose", func(c *checkout) { c.set("baselines/surface/tokens.json", "x"); c.now.Head = "b" }, nil},
		{exploreType, func(c *checkout) { c.set("notes.md", "x") }, []string{"reads and reports"}},
		{"", func(c *checkout) { c.set("CLAUDE.md", "x"); c.now.Head = "b" }, nil},
	} {
		root, c := t.TempDir(), started()
		in := hooks.Input{SessionID: "s", AgentID: "a", AgentType: tc.agentType}

		hooks.Remember(t.Context(), root, in, c.list)
		tc.change(c)

		got, err := hooks.Changes(t.Context(), root, in, c.list)
		require.NoError(t, err)
		require.Len(t, got, len(tc.want), "%q: %v", tc.agentType, got)

		for i, w := range tc.want {
			assert.Contains(t, got[i], w, tc.agentType)
		}
	}
}

// The snapshot is the checkout as the agent found it at its first tool call, not at a later one.
func TestTheSnapshotIsTakenOnce(t *testing.T) {
	t.Parallel()

	root, c := t.TempDir(), started()
	in := hooks.Input{SessionID: "s", AgentID: "a", AgentType: coderType}

	hooks.Remember(t.Context(), root, in, c.list)
	c.set("internal/tools/search/search_test.go", "x")
	hooks.Remember(t.Context(), root, in, c.list)

	got, err := hooks.Changes(t.Context(), root, in, c.list)
	require.NoError(t, err)
	assert.Len(t, got, 1, "the change after the first call is still the Coder's")
}

// A change the role may not make is red at the stop, whatever check-fast says, until it is undone;
// the stop that is green again forgets the snapshot. A reviewer runs no gate but is held to it too.
func TestAStopIsRedUntilTheChangeIsUndone(t *testing.T) {
	t.Parallel()

	for _, agentType := range []string{coderType, exploreType} {
		root, c := t.TempDir(), started()
		s := hooks.Stopper{Root: root, MaxRounds: 6, Check: (&fakeCheck{}).run, Fingerprint: same, List: c.list}
		in := hooks.Input{SessionID: "s", AgentID: "a", AgentType: agentType}

		hooks.Remember(t.Context(), root, in, c.list)
		c.set("internal/tools/search/search_test.go", "x")

		v := decode(t, s.Stop(t.Context(), in))
		assert.Equal(t, "block", v["decision"], agentType)
		assert.Contains(t, v["reason"], "search_test.go", agentType)
		assert.Contains(t, v["reason"], "git checkout HEAD -- internal/tools/search/search_test.go", "the agent is told how to undo it")

		c.set("internal/tools/search/search_test.go", "2")
		assert.Empty(t, s.Stop(t.Context(), in).Stdout, "%s: undone, the stop is green", agentType)

		c.set("internal/tools/search/search_test.go", "x")
		got, err := hooks.Changes(t.Context(), root, in, c.list)
		require.NoError(t, err)
		assert.Empty(t, got, "the green stop forgot the snapshot: a later run starts from what it finds")
	}
}

// A file the role may not write that git cannot restore (a test the Tester has not committed, for
// the Coder) is copied at the snapshot, and the report restores it from the copy, never from git,
// which would lose the Tester's work.
func TestAFileGitCannotRestoreIsRestoredFromItsCopy(t *testing.T) {
	t.Parallel()

	root, c := t.TempDir(), started()
	newTest := "internal/tools/search/new_test.go"
	writeFile(t, root, newTest, "the Tester's test\n")
	c.set(newTest, "6")
	c.now.Dirty = []string{newTest, "internal/tools/search/handler.go"}

	in := hooks.Input{SessionID: "s", AgentID: "a", AgentType: coderType}
	hooks.Remember(t.Context(), root, in, c.list)
	c.set(newTest, "x")

	got, err := hooks.Changes(t.Context(), root, in, c.list)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "cp .cache/harness/agents/")
	assert.NotContains(t, got[0], "git checkout")

	copies, err := filepath.Glob(filepath.Join(root, ".cache", "harness", "agents", "*", newTest))
	require.NoError(t, err)
	require.Len(t, copies, 1, "the Coder's test was kept, not its code")
	assert.Equal(t, "the Tester's test\n", readFile(t, copies[0]))
}

// A reviewer is sent back to undo its change for max rounds, then let go with a note: it writes
// no BLOCKED.md, which the policy refuses it, and a clean stop clears its count.
func TestAReviewerIsLetGoAfterItsRounds(t *testing.T) {
	t.Parallel()

	root, c := t.TempDir(), started()
	s := hooks.Stopper{Root: root, MaxRounds: 3, Check: (&fakeCheck{}).run, Fingerprint: same, List: c.list}
	in := hooks.Input{SessionID: "s", AgentID: "r", AgentType: exploreType}

	hooks.Remember(t.Context(), root, in, c.list)
	c.set("notes.md", "x")

	assert.Contains(t, decode(t, s.Stop(t.Context(), in))["reason"], "round 1 of 3")
	assert.Contains(t, decode(t, s.Stop(t.Context(), in))["reason"], "round 2 of 3")

	v := decode(t, s.Stop(t.Context(), in))
	assert.Nil(t, v["decision"], "let go")
	assert.Contains(t, v["systemMessage"], "tell the session")
	assert.NotContains(t, v["systemMessage"], "BLOCKED")

	delete(c.now.Files, "notes.md")
	assert.Empty(t, s.Stop(t.Context(), in).Stdout)

	hooks.Remember(t.Context(), root, in, c.list)
	c.set("notes.md", "x")
	assert.Contains(t, decode(t, s.Stop(t.Context(), in))["reason"], "round 1 of 3", "a clean stop starts the count again")
}

// A checkout that could not be read at the agent's start, or a snapshot that does not read back,
// is no pass: check-fast still runs, the stop says the change check could not, and the next tool
// call takes the snapshot again.
func TestAChangeCheckThatCannotRunIsNoPass(t *testing.T) {
	t.Parallel()

	root, c := t.TempDir(), started()
	check := &fakeCheck{}
	s := hooks.Stopper{Root: root, MaxRounds: 6, Check: check.run, Fingerprint: same, List: c.list}
	in := hooks.Input{SessionID: "s", AgentID: "a", AgentType: coderType}

	hooks.Remember(t.Context(), root, in, func(context.Context) (hooks.Snapshot, error) {
		return hooks.Snapshot{}, errors.New("git: index.lock exists")
	})

	v := decode(t, s.Stop(t.Context(), in))
	assert.Nil(t, v["decision"])
	assert.Contains(t, v["systemMessage"], "not a pass")
	assert.Contains(t, v["systemMessage"], "index.lock")
	assert.Equal(t, 1, check.calls, "check-fast ran all the same")

	hooks.Remember(t.Context(), root, in, c.list)
	c.set("internal/tools/search/search_test.go", "x")
	assert.Equal(t, "block", decode(t, s.Stop(t.Context(), in))["decision"], "the next call took the snapshot")

	snapshots, err := filepath.Glob(filepath.Join(root, ".cache", "harness", "agents", "*.json"))
	require.NoError(t, err)
	require.Len(t, snapshots, 1)
	require.NoError(t, os.WriteFile(snapshots[0], []byte(`{"head": "aa`), 0o600))

	assert.Contains(t, decode(t, s.Stop(t.Context(), in))["systemMessage"], "not a pass", "half a snapshot is no pass either")
	hooks.Remember(t.Context(), root, in, c.list)
	assert.True(t, json.Valid([]byte(readFile(t, snapshots[0]))), "and is taken again")
}

// A snapshot gone after the agent's first tool call is no pass, not "changed nothing": a later
// call does not take it again (that would hide what the agent changed before), the agent is no
// longer remembered, and a reviewer's stop says so too. An agent that made no tool call has none,
// and changed nothing.
func TestADeletedSnapshotIsNoPass(t *testing.T) {
	t.Parallel()

	root, c := t.TempDir(), started()
	s := hooks.Stopper{Root: root, MaxRounds: 6, Check: (&fakeCheck{}).run, Fingerprint: same, List: c.list}
	in := hooks.Input{SessionID: "s", AgentID: "a", AgentType: coderType}

	idle := hooks.Input{SessionID: "s", AgentID: "idle", AgentType: coderType}
	got, err := hooks.Changes(t.Context(), root, idle, c.list)
	require.NoError(t, err)
	assert.Empty(t, got, "no tool call, nothing changed")

	hooks.Remember(t.Context(), root, in, c.list)
	snapshots, err := filepath.Glob(filepath.Join(root, ".cache", "harness", "agents", "*.json"))
	require.NoError(t, err)
	require.Len(t, snapshots, 1)
	require.NoError(t, os.Remove(snapshots[0]))

	v := decode(t, s.Stop(t.Context(), in))
	assert.Contains(t, v["systemMessage"], "not a pass")
	assert.Contains(t, v["systemMessage"], "is gone")

	c.set("internal/tools/search/search_test.go", "x")
	hooks.Remember(t.Context(), root, in, c.list)
	assert.False(t, hooks.Remembered(root, in), "a later call does not take the snapshot again, which would hide the change")
	assert.Contains(t, decode(t, s.Stop(t.Context(), in))["systemMessage"], "is gone", "the stop is still no pass")

	reviewer := hooks.Input{SessionID: "s", AgentID: "r", AgentType: exploreType}
	hooks.Remember(t.Context(), root, reviewer, c.list)
	require.True(t, hooks.Remembered(root, reviewer))
	require.NoError(t, os.Remove(snapshotOf(t, root, snapshots[0])))
	r := decode(t, s.Stop(t.Context(), reviewer))
	assert.Equal(t, "block", r["decision"], "a reviewer runs no gate: an unjudged stop is held for its rounds")
	assert.Contains(t, r["reason"], "not a pass")
}

// snapshotOf is the one agent snapshot of root that is not skip.
func snapshotOf(t *testing.T, root, skip string) string {
	t.Helper()

	all, err := filepath.Glob(filepath.Join(root, ".cache", "harness", "agents", "*.json"))
	require.NoError(t, err)

	for _, p := range all {
		if p != skip {
			return p
		}
	}

	t.Fatal("no other snapshot")

	return ""
}

// An agent whose start cannot be kept (the disk refuses .cache/) is not remembered, so the
// pre-tool-use hook refuses its calls; the main session keeps no snapshot and is never refused.
func TestAStartThatCannotBeKeptIsNotRemembered(t *testing.T) {
	t.Parallel()

	root, c := t.TempDir(), started()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".cache"), []byte("a file where .cache/ should be\n"), 0o600))

	agent := hooks.Input{SessionID: "s", AgentID: "a", AgentType: coderType}
	hooks.Remember(t.Context(), root, agent, c.list)
	assert.False(t, hooks.Remembered(root, agent))
	assert.True(t, hooks.Remembered(root, hooks.Input{SessionID: "s"}), "the main session")

	v := decode(t, hooks.Deny(hooks.NotRemembered))
	assert.Contains(t, v["hookSpecificOutput"], "permissionDecision")

	ok := t.TempDir()
	hooks.Remember(t.Context(), ok, agent, c.list)
	assert.True(t, hooks.Remembered(ok, agent))
}

// A change the agent does not undo counts its red rounds like a red check-fast: after max rounds
// it must write BLOCKED.md, and then it may stop.
func TestAChangeKeptPastTheRoundsEndsBlocked(t *testing.T) {
	t.Parallel()

	root, c := t.TempDir(), started()
	s := hooks.Stopper{Root: root, MaxRounds: 2, Check: (&fakeCheck{}).run, Fingerprint: same, List: c.list}
	in := hooks.Input{SessionID: "s", AgentID: "a", AgentType: coderType}

	hooks.Remember(t.Context(), root, in, c.list)
	c.set("internal/tools/search/search_test.go", "x")

	assert.Contains(t, decode(t, s.Stop(t.Context(), in))["reason"], "round 1 of 2")
	assert.Contains(t, decode(t, s.Stop(t.Context(), in))["reason"], "red rounds: stop trying")

	writeFile(t, root, policy.BlockedFile, "the test I changed cannot be restored: the Tester's copy is gone\n")
	assert.Contains(t, decode(t, s.Stop(t.Context(), in))["systemMessage"], "stopped BLOCKED")
}

// The main session keeps no snapshot: a person is there, and it changes what it likes.
func TestTheMainSessionChangesWhatItLikes(t *testing.T) {
	t.Parallel()

	root, c := t.TempDir(), started()
	s := hooks.Stopper{Root: root, MaxRounds: 6, Check: (&fakeCheck{}).run, Fingerprint: same, List: c.list}
	in := hooks.Input{SessionID: "s"}

	hooks.Remember(t.Context(), root, in, c.list)
	c.set("CLAUDE.md", "x")
	assert.NotContains(t, s.Stop(t.Context(), in).Stdout, "CLAUDE.md", "the main session changes what it likes")
}

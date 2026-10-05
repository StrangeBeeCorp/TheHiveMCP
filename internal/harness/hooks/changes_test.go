package hooks_test

import (
	"context"
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/hooks"
)

// checkout is a fake one: its listing is what the test says it is now.
type checkout struct{ now hooks.Snapshot }

func (c *checkout) list(context.Context) (hooks.Snapshot, error) {
	return hooks.Snapshot{Head: c.now.Head, Files: maps.Clone(c.now.Files)}, nil
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
		{coderType, func(c *checkout) { c.now.Head = "bbbbbbbbbb" }, []string{"HEAD moved from aaaaaaa to bbbbbbb"}},
		{testerType, func(c *checkout) { c.set("internal/tools/search/new.go", "x") }, []string{"new.go is code"}},
		{testerType, func(c *checkout) { c.set("internal/tools/search/search_test.go", "x") }, nil},
		{"general-purpose", func(c *checkout) { c.set("CLAUDE.md", "x") }, []string{"CLAUDE.md is harness policy"}},
		{"general-purpose", func(c *checkout) { c.set("baselines/surface/tokens.json", "x"); c.now.Head = "b" }, nil},
		{"Explore", func(c *checkout) { c.set("notes.md", "x") }, []string{"reads and reports"}},
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

	for _, agentType := range []string{coderType, "Explore"} {
		root, c := t.TempDir(), started()
		s := hooks.Stopper{Root: root, MaxRounds: 6, Check: (&fakeCheck{}).run, Fingerprint: same, List: c.list}
		in := hooks.Input{SessionID: "s", AgentID: "a", AgentType: agentType}

		hooks.Remember(t.Context(), root, in, c.list)
		c.set("internal/tools/search/search_test.go", "x")

		v := decode(t, s.Stop(t.Context(), in))
		assert.Equal(t, "block", v["decision"], agentType)
		assert.Contains(t, v["reason"], "search_test.go", agentType)
		assert.Contains(t, v["reason"], "git checkout --", "the agent is told how to undo it")

		c.set("internal/tools/search/search_test.go", "2")
		assert.Empty(t, s.Stop(t.Context(), in).Stdout, "%s: undone, the stop is green", agentType)

		c.set("internal/tools/search/search_test.go", "x")
		got, err := hooks.Changes(t.Context(), root, in, c.list)
		require.NoError(t, err)
		assert.Empty(t, got, "the green stop forgot the snapshot: a later run starts from what it finds")
	}
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

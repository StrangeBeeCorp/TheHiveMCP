package hooks_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/hooks"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
)

const testerType = "tester"

func same(context.Context) (string, error) { return "unchanged", nil }

// A Tester's static run is not the Coder's: on the same checkout, the Coder's stop runs the full
// gates.
func TestAStaticRunIsNotReusedForAFullOne(t *testing.T) {
	t.Parallel()

	check := &fakeCheck{}
	s := hooks.Stopper{Root: t.TempDir(), MaxRounds: 6, Check: check.run, Fingerprint: same}

	_ = s.Stop(context.Background(), hooks.Input{SessionID: "s", AgentID: "t", AgentType: testerType})
	_ = s.Stop(context.Background(), hooks.Input{SessionID: "s", AgentID: "c", AgentType: coderType})
	_ = s.Stop(context.Background(), hooks.Input{SessionID: "s", AgentID: "c", AgentType: coderType})

	assert.Equal(t, []bool{true, false}, check.static, "the Coder's second stop reuses its own run")
}

// Each agent has its own red rounds: one agent's failures never push another to BLOCKED.
func TestEachAgentCountsItsOwnRedRounds(t *testing.T) {
	t.Parallel()

	s := hooks.Stopper{
		Root: t.TempDir(), MaxRounds: 2, Check: (&fakeCheck{result: hooks.Check{Red: true, Report: "unit: FAIL TestB"}}).run, Fingerprint: same,
	}
	first := hooks.Input{SessionID: "s", AgentID: "a1", AgentType: coderType}
	second := hooks.Input{SessionID: "s", AgentID: "a2", AgentType: coderType}

	_ = s.Stop(context.Background(), first)
	_ = s.Stop(context.Background(), first)
	assert.Contains(t, decode(t, s.Stop(context.Background(), first))["reason"], "red rounds: stop trying", "a1 is at its limit")
	assert.Contains(t, decode(t, s.Stop(context.Background(), second))["reason"], "round 1 of 2", "a2 starts its own count")
}

// A run that could not run every gate is no pass and no failure: it says so, counts no round,
// leaves a block in place, and runs again on the next stop, when the gates may run.
func TestARunThatCouldNotRunEveryGateIsRunAgain(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	blocked := filepath.Join(root, policy.BlockedFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(blocked), 0o750))
	require.NoError(t, os.WriteFile(blocked, []byte("need Docker\n"), 0o600))

	check := &fakeCheck{result: hooks.Check{Incomplete: true, Report: "lint: start OrbStack and run again"}}
	s := hooks.Stopper{Root: root, MaxRounds: 6, Check: check.run, Fingerprint: same}
	coder := hooks.Input{SessionID: "s", AgentType: coderType}

	v := decode(t, s.Stop(context.Background(), coder))
	assert.Nil(t, v["decision"], "not a block")
	assert.Contains(t, v["systemMessage"], "not a pass")
	assert.FileExists(t, blocked, "not a green run either")

	check.result = hooks.Check{}

	assert.Empty(t, s.Stop(context.Background(), coder).Stdout, "Docker is back: the gates ran, green")
	assert.Equal(t, 2, check.calls)
	assert.NoFileExists(t, blocked)
}

// A static green ends the Tester's red streak, not the task's block: only the full gates clear it.
func TestAStaticGreenLeavesTheBlockInPlace(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	blocked := filepath.Join(root, policy.BlockedFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(blocked), 0o750))
	require.NoError(t, os.WriteFile(blocked, []byte("the Coder is blocked\n"), 0o600))

	s := hooks.Stopper{Root: root, MaxRounds: 6, Check: (&fakeCheck{}).run, Fingerprint: same}
	assert.Empty(t, s.Stop(context.Background(), hooks.Input{SessionID: "s", AgentType: testerType}).Stdout)
	assert.FileExists(t, blocked)
}

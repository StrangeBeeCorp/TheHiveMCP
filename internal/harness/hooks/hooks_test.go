package hooks_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/hooks"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
)

const (
	coderType = "coder"
	filePath  = "file_path"
	grepTool  = "Grep"
	pathKey   = "path"
	patternK  = "pattern"
)

func decode(t *testing.T, out hooks.Output) map[string]any {
	t.Helper()

	if out.Stdout == "" {
		return nil
	}

	var v map[string]any
	require.NoError(t, json.Unmarshal([]byte(out.Stdout), &v))

	return v
}

func denied(t *testing.T, out hooks.Output) string {
	t.Helper()

	v := decode(t, out)
	if v == nil {
		return ""
	}

	spec, ok := v["hookSpecificOutput"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "deny", spec["permissionDecision"])

	reason, _ := spec["permissionDecisionReason"].(string)

	return reason
}

// A hook input that does not decode never passes for the main session's: the call is refused,
// and a stop is judged as an agent's.
func TestAnUnreadableHookInputIsAnAgentsAndItsToolCallIsRefused(t *testing.T) {
	t.Parallel()

	for _, data := range []string{"{", `{"tool_name": "Bash", "tool_input": {"command": "git push`, "not json"} {
		in := hooks.Parse([]byte(data))
		require.True(t, in.Unreadable, data)
		assert.Equal(t, policy.Other, in.Role(), data)

		v := decode(t, hooks.PreToolUse(t.TempDir(), in))
		spec, _ := v["hookSpecificOutput"].(map[string]any)
		assert.Equal(t, "deny", spec["permissionDecision"], data)
		assert.Contains(t, spec["permissionDecisionReason"], "could not be read", data)
	}

	assert.False(t, hooks.Parse([]byte(`{"tool_name": "Read"}`)).Unreadable)

	root := t.TempDir()
	s := hooks.Stopper{
		Root: root, MaxRounds: 6, Check: (&fakeCheck{result: hooks.Check{Red: true, Report: "unit: FAIL"}}).run,
		Fingerprint: func(context.Context) (string, error) { return "f", nil },
	}
	assert.Equal(t, "block", decode(t, s.Stop(context.Background(), hooks.Parse([]byte("{"))))["decision"],
		"a red stop sends it back, as for an agent")
}

// check-fast runs outside the state lock: a PreCompact while it runs does not wait for it.
func TestPreCompactDoesNotWaitForARunningCheckFast(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	running, release := make(chan struct{}), make(chan struct{})
	s := hooks.Stopper{
		Root: root, MaxRounds: 6,
		Check: func(context.Context, bool) (hooks.Check, error) {
			close(running)
			<-release

			return hooks.Check{}, nil
		},
		Fingerprint: func(context.Context) (string, error) { return "f", nil },
	}

	stopped := make(chan hooks.Output)

	go func() { stopped <- s.Stop(context.Background(), hooks.Input{AgentType: coderType}) }()

	<-running

	compacted := make(chan hooks.Output)

	go func() { compacted <- hooks.PreCompact(context.Background(), root) }()

	select {
	case out := <-compacted:
		assert.Empty(t, out.Stdout, "the progress was saved")
	case <-time.After(5 * time.Second):
		t.Fatal("PreCompact waited for check-fast")
	}

	close(release)
	assert.Empty(t, (<-stopped).Stdout, "a green stop says nothing")
}

func TestPreToolUseDeniesWithAReasonAndOtherwiseSaysNothing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	coder := hooks.Input{AgentType: coderType, ToolName: "Edit", ToolInput: map[string]any{filePath: filepath.Join(root, "internal/x/x_test.go")}}
	assert.Contains(t, denied(t, hooks.PreToolUse(root, coder)), "Tester")

	tester := hooks.Input{AgentType: "tester", ToolName: "Write", ToolInput: map[string]any{filePath: "internal/x/handler.go"}}
	assert.Contains(t, denied(t, hooks.PreToolUse(root, tester)), "only tests")

	push := hooks.Input{ToolName: "Bash", ToolInput: map[string]any{"command": "git push public main"}}
	assert.Contains(t, denied(t, hooks.PreToolUse(root, push)), "make pr")

	env := hooks.Input{ToolName: "Read", ToolInput: map[string]any{filePath: filepath.Join(root, ".env")}}
	assert.NotEmpty(t, denied(t, hooks.PreToolUse(root, env)))

	ok := hooks.Input{AgentType: coderType, ToolName: "Bash", ToolInput: map[string]any{"command": "make check-fast"}}
	assert.Empty(t, hooks.PreToolUse(root, ok).Stdout)
}

func TestPostToolUseSendsFindingsBack(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n"), 0o600))

	in := hooks.Input{ToolName: "Write", ToolInput: map[string]any{filePath: filepath.Join(root, "a.go")}}
	out := hooks.PostToolUse(root, in, func(rel string, _ []byte) ([]string, error) {
		return []string{rel + ": gofmt would rewrite it"}, nil
	})
	assert.Equal(t, "block", decode(t, out)["decision"])
	assert.Contains(t, decode(t, out)["reason"], "a.go: gofmt")

	clean := hooks.PostToolUse(root, in, func(string, []byte) ([]string, error) { return nil, nil })
	assert.Empty(t, clean.Stdout)
}

// The edit check reads the file an edit names, relative or absolute, and nothing outside the
// checkout or gone; a check that cannot run says so without blocking.
func TestPostToolUseChecksTheFileTheEditNames(t *testing.T) {
	t.Parallel()

	const (
		pkgA     = "pkg/a.go"
		editTool = "Edit"
	)

	root := t.TempDir()
	for _, name := range []string{pkgA, "..notes.md", "nb.ipynb"} {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("x\n"), 0o600))
	}

	var seen []string

	record := func(rel string, _ []byte) ([]string, error) {
		seen = append(seen, rel)
		return nil, nil
	}
	edit := func(key, p string) hooks.Input {
		return hooks.Input{ToolName: editTool, ToolInput: map[string]any{key: p}}
	}

	for _, in := range []hooks.Input{
		edit(filePath, pkgA),
		edit(filePath, filepath.Join(root, pkgA)),
		edit(filePath, "..notes.md"),
		edit("notebook_path", "nb.ipynb"),
		edit(filePath, filepath.Join(root, "..", "elsewhere.go")),
		edit(filePath, "/etc/hosts"),
		edit(filePath, "pkg/gone.go"),
		{ToolName: "Edit", ToolInput: map[string]any{}},
	} {
		assert.Empty(t, hooks.PostToolUse(root, in, record).Stdout)
	}

	assert.Equal(t, []string{pkgA, pkgA, "..notes.md", "nb.ipynb"}, seen, "inside the checkout and on disk only")

	broken := hooks.PostToolUse(root, edit(filePath, pkgA), func(string, []byte) ([]string, error) {
		return nil, errors.New("no gofumpt here")
	})
	assert.NotEqual(t, "block", decode(t, broken)["decision"], "a check that cannot run does not block")
	assert.Contains(t, broken.Stdout, "the file check could not run: no gofumpt here")
}

func TestDecideAppliesTheRedRoundRule(t *testing.T) {
	t.Parallel()

	assert.Equal(t, hooks.Action{}, hooks.Decide("", false, 0, 6, false, true), "green stops")

	for round := 1; round < 6; round++ {
		a := hooks.Decide("lint: FAIL", true, round, 6, false, true)
		assert.True(t, a.Block, "round %d", round)
		assert.Contains(t, a.Text, "lint: FAIL")
	}

	sixth := hooks.Decide("lint: FAIL", true, 6, 6, false, true)
	assert.True(t, sixth.Block)
	assert.Contains(t, sixth.Text, "Write "+policy.BlockedFile)

	assert.False(t, hooks.Decide("lint: FAIL", true, 7, 6, true, true).Block, "a written BLOCKED report lets it stop")
	assert.False(t, hooks.Decide("lint: FAIL", true, 0, 6, false, false).Block, "the main session is told, not held")
}

type fakeCheck struct {
	calls  int
	static []bool
	result hooks.Check
}

func (f *fakeCheck) run(_ context.Context, static bool) (hooks.Check, error) {
	f.calls++
	f.static = append(f.static, static)

	return f.result, nil
}

func TestARedStopSendsTheAgentBackUntilItWritesBlocked(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	check := &fakeCheck{result: hooks.Check{Red: true, Report: "unit: FAIL"}}
	s := hooks.Stopper{
		Root: root, MaxRounds: 6, Check: check.run,
		Fingerprint: func(context.Context) (string, error) { return "unchanged", nil },
	}
	coder := hooks.Input{SessionID: "s", AgentID: "a1", AgentType: coderType}

	for round := 1; round <= 6; round++ {
		v := decode(t, s.Stop(context.Background(), coder))
		assert.Equal(t, "block", v["decision"], "round %d", round)
	}

	assert.Equal(t, 1, check.calls, "an unchanged checkout is not checked again")

	require.NoError(t, os.WriteFile(filepath.Join(root, policy.BlockedFile), []byte("tried x, y; need z\n"), 0o600))

	v := decode(t, s.Stop(context.Background(), coder))
	assert.Nil(t, v["decision"], "BLOCKED.md written: it may stop")
	assert.Contains(t, v["systemMessage"], "BLOCKED")
}

func TestStopTreatsEachRoleItsWay(t *testing.T) {
	t.Parallel()

	fp := func(context.Context) (string, error) { return "x", nil }

	check := &fakeCheck{result: hooks.Check{Red: true, Report: "docs: FAIL"}}
	s := hooks.Stopper{Root: t.TempDir(), MaxRounds: 6, Check: check.run, Fingerprint: fp}

	v := decode(t, s.Stop(context.Background(), hooks.Input{SessionID: "s"}))
	assert.Nil(t, v["decision"], "the main session is not held back")
	assert.Contains(t, v["systemMessage"], "docs: FAIL")

	tester := &fakeCheck{result: hooks.Check{}}
	s = hooks.Stopper{Root: t.TempDir(), MaxRounds: 6, Check: tester.run, Fingerprint: fp}
	assert.Empty(t, s.Stop(context.Background(), hooks.Input{AgentType: "tester"}).Stdout)
	assert.Equal(t, []bool{true}, tester.static, "the Tester's tests fail on purpose: static gates only")

	reviewer := &fakeCheck{}
	s = hooks.Stopper{Root: t.TempDir(), MaxRounds: 6, Check: reviewer.run, Fingerprint: fp}
	assert.Empty(t, s.Stop(context.Background(), hooks.Input{AgentType: "verifier"}).Stdout)
	assert.Zero(t, reviewer.calls, "a reviewer changed nothing")
}

func TestAGreenStopResetsTheRounds(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fingerprint := "a"
	check := &fakeCheck{result: hooks.Check{Red: true, Report: "x"}}
	s := hooks.Stopper{
		Root: root, MaxRounds: 2, Check: check.run,
		Fingerprint: func(context.Context) (string, error) { return fingerprint, nil },
	}
	coder := hooks.Input{SessionID: "s", AgentType: coderType}

	_ = s.Stop(context.Background(), coder)

	fingerprint, check.result = "b", hooks.Check{}

	assert.Empty(t, s.Stop(context.Background(), coder).Stdout)

	fingerprint, check.result = "c", hooks.Check{Red: true, Report: "x"}

	assert.NotContains(t, decode(t, s.Stop(context.Background(), coder))["reason"], "red rounds: stop trying",
		"the count started again after the green stop")
}

func TestABlockedReportFromEarlierDoesNotCount(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	blocked := filepath.Join(root, policy.BlockedFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(blocked), 0o750))
	require.NoError(t, os.WriteFile(blocked, []byte("an earlier task\n"), 0o600))

	old := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(blocked, old, old))

	check := &fakeCheck{result: hooks.Check{Red: true, Report: "x"}}
	s := hooks.Stopper{Root: root, MaxRounds: 2, Check: check.run, Fingerprint: func(context.Context) (string, error) { return "f", nil }}
	coder := hooks.Input{SessionID: "s", AgentType: coderType}

	_ = s.Stop(context.Background(), coder)
	assert.Equal(t, "block", decode(t, s.Stop(context.Background(), coder))["decision"], "round 2: the old report does not count")

	require.NoError(t, os.WriteFile(blocked, []byte("this task: tried x\n"), 0o600))
	assert.Nil(t, decode(t, s.Stop(context.Background(), coder))["decision"], "a report written now lets it stop")
}

func TestAGreenStopClearsTheBlockedReport(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	blocked := filepath.Join(root, policy.BlockedFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(blocked), 0o750))
	require.NoError(t, os.WriteFile(blocked, []byte("x\n"), 0o600))

	s := hooks.Stopper{Root: root, MaxRounds: 6, Check: (&fakeCheck{}).run, Fingerprint: func(context.Context) (string, error) { return "f", nil }}
	_ = s.Stop(context.Background(), hooks.Input{SessionID: "s", AgentType: coderType})

	assert.NoFileExists(t, blocked)
}

func TestSearchesCannotReachTheEnvironmentFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	byGlob := hooks.Input{AgentType: coderType, ToolName: grepTool, ToolInput: map[string]any{patternK: "KEY", pathKey: ".", "glob": ".env*"}}
	assert.NotEmpty(t, denied(t, hooks.PreToolUse(root, byGlob)))

	byPattern := hooks.Input{ToolName: "Glob", ToolInput: map[string]any{patternK: "**/.env.local"}}
	assert.NotEmpty(t, denied(t, hooks.PreToolUse(root, byPattern)))

	canaries := hooks.Input{AgentType: coderType, ToolName: grepTool, ToolInput: map[string]any{patternK: "x", pathKey: "gates/canaries"}}
	assert.NotEmpty(t, denied(t, hooks.PreToolUse(root, canaries)), "the directory itself, not only its files")

	code := hooks.Input{AgentType: coderType, ToolName: grepTool, ToolInput: map[string]any{patternK: ".env", pathKey: "internal"}}
	assert.Empty(t, hooks.PreToolUse(root, code).Stdout, "searching code for the word is fine")
}

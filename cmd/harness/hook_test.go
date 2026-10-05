package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/hooks"
)

// editRegistry runs the structure and public-safety builtins on each edit, as gates/registry.yaml
// does, with limits that list the top-level entries and refuse the sealed tests in the repository.
const (
	editRegistry = `
scopes: {edit: a, stop: a, commit: a, pr: a, ci: a, nightly: a}
suites:
  - {id: everything, review: high, paths: ["**"]}
gates:
  - {id: layout, builtin: structure, cadence: [edit], suites: ["*"], budget_s: 10, blocks: x}
  - {id: safety, builtin: public-safety, cadence: [edit], suites: ["*"], budget_s: 10, blocks: x}
`
	editLimits = `structure: {max_lines: 500, root: [.gitignore, docs, gates, internal]}
public_safety: {forbidden_paths: [".claude/sealed/**"]}
hooks: {max_red_rounds: 6}
`
)

const gitignore = ".gitignore"

// The files of the change-check test: a package, and a test the Tester has not committed.
const (
	pkgX      = "package x\n"
	newTestGo = "internal/x/new_test.go"
	xGo       = "internal/x/x.go"
)

// hook runs bin/harness hook <event> with the input as JSON on stdin, and decodes its answer.
func hook(t *testing.T, event string, input map[string]any) map[string]any {
	t.Helper()

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var out strings.Builder
	require.NoError(t, dispatch(t.Context(), []string{"hook", event}, strings.NewReader(string(data)), &out))

	if out.Len() == 0 {
		return nil
	}

	var v map[string]any
	require.NoError(t, json.Unmarshal([]byte(out.String()), &v))

	return v
}

// The edit check judges what reaches the repository: a file git ignores (BLOCKED.md, a scratch
// file, the Tester's sealed tests) is not checked, and a new top-level entry is.
//
//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestTheEditCheckSkipsWhatGitIgnores(t *testing.T) {
	dir := gittest.Repo(t, map[string]string{
		registryPath: editRegistry, limitsPath: editLimits, gitignore: ".cache/\n.claude/sealed/\n", "docs/a.md": "# notes\n",
	})
	t.Chdir(dir)

	written := func(rel string) map[string]any {
		gittest.Write(t, dir, map[string]string{rel: "x\n"})

		return hook(t, "post-tool-use", map[string]any{"tool_name": "Write", "tool_input": map[string]any{"file_path": filepath.Join(dir, rel)}})
	}

	for _, rel := range []string{".cache/harness/BLOCKED.md", ".cache/tmp/notes.txt", ".claude/sealed/DL-1/internal/x_test.go"} {
		assert.Nil(t, written(rel), rel)
	}

	v := written("newdir/x.md")
	assert.Equal(t, "block", v["decision"])
	assert.Contains(t, v["reason"], `top-level entry "newdir"`)
}

// The change check runs through the real hook and git: a Coder's first tool call keeps the
// checkout, a test it then changes by any command sends its stop back with the command that
// restores it (from git for a committed test, from a copy for one the Tester has not committed),
// and once both are restored the stop is green.
//
// An agent's tool call is refused when the hook cannot keep the checkout as the agent found it:
// its stop could not judge what it changed. The main session keeps none and goes on.
//
//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestAnAgentWhoseStartCannotBeKeptIsRefused(t *testing.T) {
	dir := gittest.Repo(t, map[string]string{
		registryPath: editRegistry, limitsPath: editLimits, gitignore: ".cache\n", xGo: pkgX,
	})
	gittest.Write(t, dir, map[string]string{".cache": "a file where .cache/ should be\n"})
	t.Chdir(dir)

	call := map[string]any{
		"session_id": "s", "agent_id": "c1", "agent_type": "coder",
		"tool_name": "Bash", "tool_input": map[string]any{"command": "make test"},
	}
	v := hook(t, "pre-tool-use", call)
	require.NotNil(t, v)
	assert.Contains(t, fmt.Sprint(v["hookSpecificOutput"]), hooks.NotRemembered)

	delete(call, "agent_id")
	delete(call, "agent_type")
	assert.Nil(t, hook(t, "pre-tool-use", call), "the main session")
}

//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestACoderThatChangesATestIsSentBack(t *testing.T) {
	dir := gittest.Repo(t, map[string]string{
		registryPath: editRegistry, limitsPath: editLimits, gitignore: ".cache/\n.claude/sealed/\n",
		xGo: pkgX, "internal/x/x_test.go": pkgX,
	})
	gittest.Write(t, dir, map[string]string{newTestGo: "package x // the Tester's\n"})
	t.Chdir(dir)

	coder := map[string]any{"session_id": "s", "agent_id": "c1", "agent_type": "coder"}
	call := maps.Clone(coder)
	call["tool_name"], call["tool_input"] = "Bash", map[string]any{"command": "make test"}
	assert.Nil(t, hook(t, "pre-tool-use", call))

	gittest.Write(t, dir, map[string]string{
		"internal/x/x_test.go": "package x // weakened\n", newTestGo: "package x // weakened\n", xGo: "package x // done\n",
	})

	v := hook(t, "subagent-stop", coder)
	require.NotNil(t, v)
	assert.Equal(t, "block", v["decision"])

	reason, _ := v["reason"].(string)
	assert.Contains(t, reason, "git checkout HEAD -- internal/x/x_test.go")
	assert.Contains(t, reason, "cp .cache/harness/agents/")
	assert.NotContains(t, reason, "internal/x/x.go ", "the code is the Coder's")

	gittest.Git(t, dir, "checkout", "HEAD", "--", "internal/x/x_test.go")

	copies, err := filepath.Glob(filepath.Join(dir, ".cache", "harness", "agents", "*", "internal", "x", "new_test.go"))
	require.NoError(t, err)
	require.Len(t, copies, 1)

	kept, err := os.ReadFile(copies[0]) // #nosec G304 -- the copy the hook kept
	require.NoError(t, err)
	gittest.Write(t, dir, map[string]string{newTestGo: string(kept)})

	assert.Nil(t, hook(t, "subagent-stop", coder), "restored: the stop is green")
}

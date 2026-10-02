package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"
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

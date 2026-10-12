package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"
)

// The dashboard reads the checkout as the structure gate does: the files git would commit, the
// largest first, as a table or as JSON.
//
//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestTheStructureDashboardRanksTheCheckout(t *testing.T) {
	limits := "structure: {max_lines: 500, max_code_files_per_dir: 14, code_files: [\"*.go\"], root: [.gitignore, gates, internal]}\n" +
		"hooks: {max_red_rounds: 6}\n"
	dir := gittest.Repo(t, map[string]string{
		registryPath: editRegistry, limitsPath: limits, gitignore: ignoreCache,
		"internal/x/long.go": "package x\n" + strings.Repeat("// x\n", 20), "internal/x/short.go": pkgX,
	})
	gittest.Write(t, dir, map[string]string{".cache/ignored.go": strings.Repeat("// x\n", 900)})
	t.Chdir(dir)

	var out strings.Builder
	require.NoError(t, dispatch(t.Context(), []string{builtinStructure, "--json"}, strings.NewReader(""), &out))

	var sizes checks.Sizes
	require.NoError(t, json.Unmarshal([]byte(out.String()), &sizes))
	require.Len(t, sizes.Files, 2, "an ignored file is not the repository's")
	assert.Equal(t, "internal/x/long.go", sizes.Files[0].Path)
	assert.Equal(t, 21, sizes.Files[0].Lines)

	out.Reset()
	require.NoError(t, dispatch(t.Context(), []string{builtinStructure, "--top", "1"}, strings.NewReader(""), &out))
	assert.Contains(t, out.String(), "0 code files over 500 lines")
	assert.Contains(t, out.String(), "internal/x/long.go")
	assert.NotContains(t, out.String(), "short.go", "--top 1")

	var usage usageError
	require.ErrorAs(t, dispatch(t.Context(), []string{builtinStructure, "--top", "0"}, strings.NewReader(""), &out), &usage)
}

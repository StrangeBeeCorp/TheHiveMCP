package surface_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/surface"
)

func TestTokensAreCharactersOverFour(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 0, surface.Tokens(nil))
	assert.Equal(t, 1, surface.Tokens([]byte("abcd")))
	assert.Equal(t, 2, surface.Tokens([]byte("abcde")))
	assert.Equal(t, 1, surface.Tokens([]byte("—→")), "characters, not bytes")
}

func TestTakeReadsTheServersSurface(t *testing.T) {
	t.Parallel()

	snap, err := surface.Take(t.Context())
	require.NoError(t, err)

	names := make([]string, 0, len(snap.Tools))
	for _, tool := range snap.Tools {
		names = append(names, tool.Name)
		assert.Positive(t, tool.Tokens, tool.Name)
	}

	assert.Equal(t, []string{"execute-automation", "get-resource", "manage-entities", "search-entities"}, names)
	assert.NotEmpty(t, snap.Resources)

	read := 0

	for _, r := range snap.Resources {
		if r.SHA256 != "" {
			read++
		}
	}

	assert.Positive(t, read, "the static resource texts are read and hashed")
}

func TestASnapshotRoundTripsAndDiffsByFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	files := map[string][]byte{"tools/a.json": []byte("{}\n"), "tokens.json": []byte("{}\n")}
	require.NoError(t, surface.Write(dir, files))

	drift, err := surface.Diff(dir, files)
	require.NoError(t, err)
	assert.Empty(t, drift)

	changed := map[string][]byte{"tools/a.json": []byte(`{"x":1}` + "\n"), "tools/b.json": []byte("{}\n")}
	drift, err = surface.Diff(dir, changed)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"tokens.json: in the snapshot, no longer advertised",
		"tools/a.json: changed",
		"tools/b.json: new, not in the snapshot",
	}, drift)

	require.NoError(t, surface.Write(dir, changed))
	assert.NoFileExists(t, filepath.Join(dir, "tokens.json"), "a file no longer produced is removed")
}

func TestBudgetsNameWhatIsOver(t *testing.T) {
	t.Parallel()

	b := surface.Budgets{ToolsTotal: 100, Tool: 60, ResourcesTotal: 10}
	assert.Empty(t, b.Over(surface.Summary{Tools: map[string]int{"a": 50, "b": 50}, ToolsTotal: 100, ResourcesTotal: 10}))

	over := b.Over(surface.Summary{Tools: map[string]int{"a": 61, "b": 50}, ToolsTotal: 111, ResourcesTotal: 11})
	assert.Len(t, over, 3)
	assert.Contains(t, over[1], "a costs 61 tokens")

	p := filepath.Join(t.TempDir(), "budgets.yaml")
	require.NoError(t, os.WriteFile(p, []byte("surface:\n  tools_total: 1\n"), 0o600))
	_, err := surface.LoadBudgets(p)
	assert.Error(t, err, "every budget is required")
}

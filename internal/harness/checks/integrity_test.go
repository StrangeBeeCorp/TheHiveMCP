package checks_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
)

const (
	suiteFile = "pkg/b_test.go"
	otherFile = "pkg/c_test.go"
	baseTest  = `package pkg

import "testing"

func TestSearch(t *testing.T) {
	for _, c := range []string{"a", "b", "c"} {
		require.NoError(t, run(c))
		assert.Equal(t, 1, count(c))
	}
	if broken() {
		t.Fatal("broken")
	}
	t.Run("sub", func(t *testing.T) { requireFound(t, "d") })
}

func helper() {}
`
)

// integrity runs the check on suiteFile changed from baseTest to now ("" deletes it), plus other
// files of the tree as they are.
func integrity(t *testing.T, now string, others checks.MemTree, allow map[string]string) []string {
	t.Helper()

	tree := checks.MemTree{}
	changed := []string{suiteFile}

	if now != "" {
		tree[suiteFile] = []byte(now)
	}

	for p, data := range others {
		tree[p] = data
		changed = append(changed, p)
	}

	before := func(p string) ([]byte, bool) { return []byte(baseTest), p == suiteFile }

	found, err := checks.Integrity(tree, changed, before, checks.IntegrityLimits{Allow: allow})
	require.NoError(t, err)

	lines := make([]string, len(found))
	for i, f := range found {
		lines[i] = f.String()
	}

	return lines
}

func TestAnUnchangedOrStrongerTestPasses(t *testing.T) {
	t.Parallel()

	assert.Empty(t, integrity(t, baseTest, nil, nil))

	stronger := edit(t, `{"a", "b", "c"}`, `{"a", "b", "c", "d"}`)
	assert.Empty(t, integrity(t, stronger, nil, nil), "a case added")
	assert.Empty(t, integrity(t, edit(t, "func helper() {}", ""), nil, nil), "a helper is not a test")
}

func TestAWeakenedTestIsRefused(t *testing.T) {
	t.Parallel()

	for name, c := range map[string]struct{ now, want string }{
		"an assertion removed":  {edit(t, "\t\tassert.Equal(t, 1, count(c))\n", ""), "TestSearch checks less: 3 assertions, was 4"},
		"a helper call removed": {edit(t, `requireFound(t, "d")`, ""), "TestSearch checks less: 3 assertions, was 4"},
		"a skip added":          {edit(t, "func TestSearch(t *testing.T) {\n", "func TestSearch(t *testing.T) {\n\tt.Skip(\"flaky\")\n"), "TestSearch now skips (1 skips, was 0)"},
		"a case removed":        {edit(t, `{"a", "b", "c"}`, `{"a", "b"}`), "TestSearch has fewer cases: 2, was 3"},
		"the test removed":      {"package pkg\n", "TestSearch is gone"},
		"the file removed":      {"", "TestSearch is gone"},
	} {
		found := integrity(t, c.now, nil, nil)
		require.Len(t, found, 1, name)
		assert.Contains(t, found[0], c.want, name)
		assert.Contains(t, found[0], "gates/limits.yaml (integrity.allow)", name)
	}
}

func TestAMovedTestIsComparedWhereItWent(t *testing.T) {
	t.Parallel()

	moved := checks.MemTree{otherFile: []byte(edit(t, "func helper() {}", ""))}
	assert.Empty(t, integrity(t, "package pkg\n", moved, nil))

	weakened := checks.MemTree{otherFile: []byte(edit(t, "\t\tassert.Equal(t, 1, count(c))\n", ""))}
	assert.Len(t, integrity(t, "package pkg\n", weakened, nil), 1, "moved and weakened")
}

func TestAPersonMayAcceptAWeakerTest(t *testing.T) {
	t.Parallel()

	allow := map[string]string{suiteFile + ":TestSearch": "the search API dropped this case"}
	assert.Empty(t, integrity(t, "package pkg\n", nil, allow))
}

func TestANewTestFileHasNothingToCompare(t *testing.T) {
	t.Parallel()

	tree := checks.MemTree{otherFile: []byte(baseTest), "pkg/impl.go": []byte("package pkg\n")}
	never := func(string) ([]byte, bool) { return nil, false }

	found, err := checks.Integrity(tree, []string{otherFile, "pkg/impl.go"}, never, checks.IntegrityLimits{})
	require.NoError(t, err)
	assert.Empty(t, found)
}

// edit is baseTest with old replaced by repl.
func edit(t *testing.T, old, repl string) string {
	t.Helper()
	require.Contains(t, baseTest, old)

	return strings.Replace(baseTest, old, repl, 1)
}

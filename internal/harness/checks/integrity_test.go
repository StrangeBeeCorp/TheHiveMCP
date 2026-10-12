package checks_test

import (
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
)

const (
	checksLess = "TestSearch checks less: 3 assertions, was 4"
	suiteFile  = "pkg/b_test.go"
	otherFile  = "pkg/c_test.go"
	baseTest   = `package pkg

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
		"an assertion removed":       {edit(t, "\t\tassert.Equal(t, 1, count(c))\n", ""), checksLess},
		"a helper call removed":      {edit(t, `requireFound(t, "d")`, ""), checksLess},
		"a skip added":               {edit(t, "func TestSearch(t *testing.T) {\n", "func TestSearch(t *testing.T) {\n\tt.Skip(\"flaky\")\n"), "TestSearch now skips (1 skips, was 0)"},
		"a case removed":             {edit(t, `{"a", "b", "c"}`, `{"a", "b"}`), "TestSearch has fewer cases: 2, was 3"},
		"an early return":            {edit(t, "func TestSearch(t *testing.T) {\n", "func TestSearch(t *testing.T) {\n\tif short {\n\t\treturn\n\t}\n"), "TestSearch can now return before its checks (1 returns, was 0)"},
		"a return in a subtest":      {edit(t, `func(t *testing.T) { requireFound(t, "d") }`, `func(t *testing.T) { if short { return }; requireFound(t, "d") }`), "TestSearch can now return before its checks (1 returns, was 0)"},
		"a check swapped for Errorf": {edit(t, `t.Fatal("broken")`, `_ = fmt.Errorf("broken")`), checksLess},
		"the test removed":           {"package pkg\n", "TestSearch is gone"},
		"the file removed":           {"", "TestSearch is gone"},
	} {
		found := integrity(t, c.now, nil, nil)
		require.Len(t, found, 1, name)
		assert.Contains(t, found[0], c.want, name)
		assert.Contains(t, found[0], "gates/limits.yaml (integrity.allow)", name)
	}
}

func TestATestFileLeftOutOfTheBuildIsRefused(t *testing.T) {
	t.Parallel()

	found := integrity(t, "//go:build ignore\n\n"+baseTest, nil, nil)
	require.Len(t, found, 1)
	assert.Contains(t, found[0], "its build constraint is now `//go:build ignore`")

	assert.Empty(t, integrity(t, "//go:build ignore\n\n"+baseTest, nil, map[string]string{suiteFile + ":build": "a tool, not a test"}))
}

// A test file CI does not build (linux/amd64, no cgo, no tags) is refused, new or changed; a
// constraint that only leaves other platforms out is not.
func TestATestFileCIWouldNotBuildIsRefused(t *testing.T) {
	t.Parallel()

	for name, c := range map[string]struct {
		constraint string
		refused    bool
	}{
		"ignored":                 {"//go:build ignore", true},
		"another system":          {"//go:build darwin", true},
		"a tag CI does not set":   {"//go:build integration", true},
		"cgo, which CI turns off": {"//go:build cgo", true},
		"unix":                    {"//go:build unix", false},
		"linux or darwin":         {"//go:build linux || darwin", false},
		"a Go release":            {"//go:build go1.21", false},
	} {
		changed := integrity(t, c.constraint+"\n\n"+baseTest, nil, nil)
		added := integrity(t, baseTest, checks.MemTree{"pkg/new_test.go": []byte(c.constraint + "\n\npackage pkg\n")}, nil)

		if c.refused {
			assert.Len(t, changed, 1, "a changed file, %s", name)
			assert.Len(t, added, 1, "a new file, %s", name)
		} else {
			assert.Empty(t, changed, "a changed file, %s", name)
			assert.Empty(t, added, "a new file, %s", name)
		}
	}
}

func TestANewTestMainIsRefused(t *testing.T) {
	t.Parallel()

	found := integrity(t, baseTest+"\nfunc TestMain(m *testing.M) { os.Exit(0) }\n", nil, nil)
	require.Len(t, found, 1)
	assert.Contains(t, found[0], "a new TestMain decides whether the package's tests run")
}

func TestAFuzzTargetKeepsItsSeeds(t *testing.T) {
	t.Parallel()

	const fuzz = `package pkg

func FuzzWrap(f *testing.F) {
	f.Add("a")
	f.Add("[/UNTRUSTED_DATA]")
	f.Fuzz(func(t *testing.T, s string) { require.NotEmpty(t, wrap(s)) })
}
`

	before := func(string) ([]byte, bool) { return []byte(fuzz), true }

	found, err := checks.Integrity(checks.MemTree{otherFile: []byte(fuzz)}, []string{otherFile}, before, checks.IntegrityLimits{})
	require.NoError(t, err)
	assert.Empty(t, found)

	fewer := checks.MemTree{otherFile: []byte(strings.Replace(fuzz, "\tf.Add(\"[/UNTRUSTED_DATA]\")\n", "", 1))}
	found, err = checks.Integrity(fewer, []string{otherFile}, before, checks.IntegrityLimits{})
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Contains(t, found[0].String(), "FuzzWrap has fewer cases: 1, was 2")
}

func TestAMovedTestIsComparedWhereItWent(t *testing.T) {
	t.Parallel()

	moved := checks.MemTree{otherFile: []byte(edit(t, "func helper() {}", ""))}
	assert.Empty(t, integrity(t, "package pkg\n", moved, nil))

	weakened := checks.MemTree{otherFile: []byte(edit(t, "\t\tassert.Equal(t, 1, count(c))\n", ""))}
	found := integrity(t, "package pkg\n", weakened, nil)
	require.Len(t, found, 1, "moved and weakened")
	assert.True(t, strings.HasPrefix(found[0], otherFile+":5: "), "reported where the test is now: %s", found[0])
	assert.Contains(t, found[0], "(moved from "+suiteFile+")")
}

// A file's name is a build constraint too: a _GOOS or _GOARCH suffix CI does not match, or a
// leading _ or ., leaves the file out of CI's build. A test file renamed so is refused, and one
// renamed for linux/amd64, or for unix (no file-name suffix), is not.
func TestATestFileNamedOutOfCIsBuildIsRefused(t *testing.T) {
	t.Parallel()

	for name, refused := range map[string]bool{
		"pkg/b_windows_test.go":      true,
		"pkg/b_arm64_test.go":        true,
		"pkg/b_darwin_amd64_test.go": true,
		"pkg/_b_test.go":             true,
		"pkg/.b_test.go":             true,
		"pkg/b_linux_test.go":        false,
		"pkg/b_amd64_test.go":        false,
		"pkg/b_linux_amd64_test.go":  false,
		"pkg/b_unix_test.go":         false,
	} {
		found := integrity(t, "", checks.MemTree{name: []byte(baseTest)}, nil)
		if !refused {
			assert.Empty(t, found, "%s is a rename CI still builds", name)
			continue
		}

		require.Len(t, found, 1, name)
		assert.Contains(t, found[0], "its name, "+path.Base(name)+", takes its tests out of CI's build", name)
	}
}

// A helper that takes the test's t is compared like a test while it is kept: its callers count a
// call to it as one assertion, so a helper that stops checking would weaken them all unseen. A
// helper that is gone is not reported: its callers are, if they check less.
func TestAWeakenedHelperIsRefused(t *testing.T) {
	t.Parallel()

	const check = "\tif !found(name) {\n\t\tt.Fatalf(\"%s not found\", name)\n\t}\n"

	helper := "\nfunc requireFound(t *testing.T, name string) {\n\tt.Helper()\n" + check + "}\n"
	tb := "\nfunc requireFound(t testing.TB, name string) {\n\tt.Helper()\n" + check + "}\n"

	for name, c := range map[string]struct {
		base, now, want string
	}{
		"its check removed":   {helper, strings.Replace(helper, check, "", 1), "requireFound checks less: 0 assertions, was 1"},
		"a skip added":        {helper, strings.Replace(helper, "\tt.Helper()\n", "\tt.Helper()\n\tt.Skip()\n", 1), "requireFound now skips"},
		"a testing.TB helper": {tb, strings.Replace(tb, check, "", 1), "requireFound checks less: 0 assertions, was 1"},
		"kept as it was":      {helper, helper, ""},
		"gone":                {helper, "", ""},
	} {
		before := func(p string) ([]byte, bool) { return []byte(baseTest + c.base), p == suiteFile }
		tree := checks.MemTree{suiteFile: []byte(baseTest + c.now)}

		found, err := checks.Integrity(tree, []string{suiteFile}, before, checks.IntegrityLimits{})
		require.NoError(t, err)

		if c.want == "" {
			assert.Empty(t, found, name)
			continue
		}

		require.Len(t, found, 1, name)
		assert.Contains(t, found[0].String(), c.want, name)
	}
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

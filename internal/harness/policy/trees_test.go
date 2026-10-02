package policy_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
)

// checkoutWith is a checkout on disk with these files, for the rules that look at what a path
// holds, and a judge of its command lines.
func checkoutWith(t *testing.T, files ...string) (string, func(policy.Role, string) policy.Decision) {
	t.Helper()

	root := t.TempDir()
	for _, f := range files {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, f)), 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(root, f), []byte("x\n"), 0o600))
	}

	return root, func(role policy.Role, line string) policy.Decision { return policy.Bash(role, root, root, line) }
}

const (
	searchTest = "internal/tools/search/search_test.go"
	searchCode = "internal/tools/search/handler.go"
	harnessGo  = "cmd/harness/main.go"
)

// A move takes its source away, a copy into a directory writes under it, and rm -r, chmod -R and
// a pattern reach every file they match: each such file is judged, not only the path named.
func TestEveryFileACommandReachesIsJudged(t *testing.T) {
	t.Parallel()

	fixtures := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(fixtures, "harness"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(fixtures, "harness", "x.go"), []byte("x\n"), 0o600))

	_, in := checkoutWith(t, searchTest, searchCode, harnessGo, ".cache/tmp/a.txt", "internal/tools/search/testdata/q.json",
		".cache/tmp/clone/.git/HEAD", ".cache/tmp/clone/README.md")

	for _, line := range []string{
		"mv " + searchTest + " .cache/tmp/",
		"mv -t .cache/tmp " + searchTest,
		"mv --target-directory=.cache/tmp " + searchTest,
		"mv internal/tools/search/* .cache/tmp/",
		"rm -rf internal/tools/search",
		"rm -r internal/tools/search/*",
		"rm internal/tools/search/*",
		"rm -rf internal/tools/search/testdata",
		"cp /tmp/x_test.go internal/tools/search/",
		"cp -t internal/tools/search /tmp/x_test.go",
		"cp /tmp/x_test.go --target-directory=internal/tools/search",
		"cp -r " + fixtures + " internal/tools/search/testdata",
	} {
		assert.True(t, in(policy.Coder, line).Deny, "Coder: %s", line)
	}

	for _, line := range []string{
		"mv " + harnessGo + " .cache/tmp/",
		"rm -rf cmd",
		"chmod -R 777 cmd",
		"cp -r " + fixtures + "/ internal/",
		"ln -s /tmp/main.go cmd/harness/",
	} {
		assert.True(t, in(policy.Other, line).Deny, "Other: %s", line)
	}

	assert.True(t, in(policy.Tester, "mv "+searchCode+" .cache/tmp/").Deny, "the Tester does not move code away")

	for _, line := range []string{
		"rm -rf .cache/tmp/clone",
		"mv .cache/tmp/a.txt .cache/tmp/b.txt",
		"cp " + searchCode + " .cache/tmp/",
		"rm -f .cache/tmp/*",
		"cp -r " + fixtures + " .cache/tmp/fixtures",
	} {
		assert.False(t, in(policy.Coder, line).Deny, "Coder: %s: %v", line, in(policy.Coder, line).Reason)
	}

	assert.False(t, in(policy.Main, "rm -rf cmd").Deny, "the main session changes the harness")
}

// A write to a path the shell computes as the line runs is judged when it is ${TMPDIR}/…, where
// the agents put their logs, and refused to an agent anywhere else.
func TestAComputedWriteIsJudgedOnlyUnderTMPDIR(t *testing.T) {
	t.Parallel()

	for _, role := range []policy.Role{policy.Tester, policy.Coder, policy.Other} {
		assert.False(t, bash(role, `make test-integration RUN=TestSearch > "$TMPDIR/it.log" 2>&1`).Deny, "%s", role)

		for _, line := range []string{
			`echo x > "$(git rev-parse --show-toplevel)/cmd/harness/main.go"`,
			`echo x > "$HOME/x.go"`,
			`echo x | tee "$D/main.go"`,
			`TMPDIR=cmd/harness; echo x > "$TMPDIR/main.go"`,
		} {
			assert.True(t, bash(role, line).Deny, "%s: %s", role, line)
		}
	}

	assert.False(t, bash(policy.Main, `echo x > "$S/notes.txt"`).Deny, "the main session's computed paths are its own")
}

// A cd holds for the rest of its shell only: one in a child shell, ( ) or sh -c or git rebase
// --exec, leaves the line where it was, and one the policy cannot follow leaves relative paths
// refused.
func TestADirectoryChangeHoldsOnlyInItsShell(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"sh -c 'cd .cache/tmp'; echo x > internal/x_test.go",
		"(cd .cache/tmp); echo x > x_test.go",
		"cd .cache/tmp | true; echo x > x_test.go",
		"pushd internal/tools/search/testdata && echo x > q.json",
		"pushd .cache/tmp && popd && echo x > x_test.go",
		"cd - && echo x > x_test.go",
		"CDPATH=internal cd tools",
	} {
		assert.True(t, bash(policy.Coder, line).Deny, "Coder: %s", line)
	}

	assert.True(t, bash(policy.Other, "git rebase -x 'cd .cache/tmp' HEAD~1; echo x > cmd/harness/main.go").Deny)
	assert.False(t, bash(policy.Coder, "bash -c 'cd .cache/tmp && echo x > a_test.go'").Deny, "inside its shell, the cd holds")
	assert.False(t, bash(policy.Coder, "cd .cache/tmp && echo x > a_test.go").Deny)
}

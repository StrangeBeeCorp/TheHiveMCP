package policy_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
)

// A path is judged where it lands: /dev/.. leaves /dev, and an absolute target is judged even
// after a cd the policy could not follow.
func TestAPathIsJudgedWhereItLandsWhateverTheLineDidBefore(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"echo x > /dev/../r/thehivemcp/.cache/harness/evidence/abc.json",
		"tee /dev/fd/../../r/thehivemcp/.cache/harness/state.json < /tmp/x",
		`cd "$TMPDIR" && tar -xf "$TMPDIR/e.tar" -C /r/thehivemcp/.cache`,
		"cd - && unzip -o /tmp/e.zip -d /r/thehivemcp/.cache/harness",
		"cd - && cp -r /tmp/evidence /r/thehivemcp/.cache/harness",
	} {
		assert.True(t, bash(policy.Other, line).Deny, line)
	}

	assert.True(t, bash(policy.Main, "echo x > /dev/../r/thehivemcp/.git/config").Deny, "git's own files are no one's")
	assert.False(t, bash(policy.Other, "echo x > /dev/null").Deny)
	assert.False(t, bash(policy.Other, "make test 2> /dev/stderr").Deny)
}

// gawk takes its program from -e and --source, and from a file with -f, -E, --exec, -W exec= or
// @include, and its long options cut short: the policy reads the program wherever it is, and an
// agent's program is inline.
func TestAnAwkProgramIsReadWhereverItsOptionsPutIt(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		`gawk --assign a=1 'BEGIN{print "x" > "spec/a.md"}'`,
		`gawk -e 'BEGIN{}' -e 'BEGIN{print > "f"}'`,
		`gawk --source='BEGIN{system("gh pr merge 1")}'`,
		"gawk -E .cache/tmp/p.awk",
		"gawk --exec .cache/tmp/p.awk",
		"gawk --fil=.cache/tmp/p.awk",
		"gawk -W exec=.cache/tmp/p.awk",
		`gawk '@include ".cache/tmp/p.awk"'`,
		"gawk --dump-variables=x '{print}' f",
		"gawk -o '{print}' f",
	} {
		assert.True(t, bash(policy.Other, line).Deny, line)
	}

	for _, line := range []string{"awk -F, '{print $1}' f", "awk -v n=1 '{print n}' f", "gawk -e '{print $2}' f", "awk -- '{print}' f"} {
		assert.False(t, bash(policy.Other, line).Deny, line)
	}

	assert.True(t, bash(policy.Main, `gawk -e 'BEGIN{system("git push")}'`).Deny, "the command an -e program runs is judged")
	assert.True(t, bash(policy.Main, `gawk --assign a=1 'BEGIN{system("git push")}'`).Deny, "--assign takes the next word, not the program")
	assert.True(t, bash(policy.Main, "gawk -i inplace '{print}' .git/config").Deny, "inplace rewrites its operands")
	assert.False(t, bash(policy.Main, "gawk -i inplace '{print}' notes.md").Deny)
}

// What sed writes with w and s///w is judged as a write of the role: only the target tells these
// apart, so the parser alone decides.
func TestWhatSedWritesIsJudgedAsTheRolesWrite(t *testing.T) {
	t.Parallel()

	assert.True(t, bash(policy.Coder, "sed -n 'w internal/x_test.go' f").Deny, "the Coder writes no test")
	assert.False(t, bash(policy.Coder, "sed -n 'w internal/x.go' f").Deny)
	assert.True(t, bash(policy.Tester, "sed 's/a/b/w internal/x.go' f").Deny, "the Tester writes only tests")
	assert.False(t, bash(policy.Tester, "sed 's/a/b/w internal/x_test.go' f").Deny)
}

// An alias, an extension or a git-<name> program runs with words the policy would read as another
// command's: only git's and gh's own commands are judged, and git subtree's push is a push.
func TestAnAliasOrAnExtensionDoesNotStandInForACommand(t *testing.T) {
	t.Parallel()

	for _, role := range []policy.Role{policy.Main, policy.Other} {
		for _, line := range []string{
			"git h push --person",
			"git g push origin HEAD:main",
			"git lfs push origin main",
			"gh p merge 1",
			"gh co 1",
			"git subtree push --prefix=docs origin main",
			"git subtree --prefix=docs push origin main",
			"git http-push origin main",
		} {
			assert.True(t, bash(role, line).Deny, "%s: %s", role, line)
		}
	}

	assert.True(t, bash(policy.Other, "git subtree split --prefix=docs").Deny, "an agent rewrites no history")
	assert.False(t, bash(policy.Main, "git subtree split --prefix=docs").Deny)
	assert.False(t, bash(policy.Main, "git status").Deny)
	assert.False(t, bash(policy.Other, "gh pr view 1").Deny)
}

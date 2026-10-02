package policy_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
)

// What a runner hands a command (xargs's input, find's {}, parallel's :::, a shell's script and
// its arguments, git's filters and transports) is read as the command it completes.
func TestWhatACommandIsHandedIsReadToo(t *testing.T) {
	t.Parallel()

	agents := []policy.Role{policy.Tester, policy.Coder, policy.Other}

	for _, line := range []string{
		"echo push origin HEAD:main | xargs git",
		"echo 'pr merge 5' | xargs gh",
		"echo merge 5 | xargs gh pr",
		"echo pr | xargs make",
		"echo x | xargs -I{} git {} origin HEAD:main",
		"echo repos/o/r/git/refs/heads/main | xargs gh api -X DELETE",
		"find push -maxdepth 0 -exec git {} origin HEAD:main \\;",
		"parallel git ::: push",
	} {
		for _, role := range append(slices.Clone(agents), policy.Main) {
			assert.True(t, bash(role, line).Deny, "%s: %s", role, line)
		}
	}

	for _, line := range []string{
		"bash bin/harness push --person",
		"sh bin/harness pr --person",
		"bash -- bin/harness push",
		"bash scripts/ship.sh --person",
		"git filter-branch -f --msg-filter 'bin/harness push --person; cat' HEAD^..HEAD",
		"git send-email --to-cmd='bin/harness push' HEAD^",
		"git fetch --upload-pack='bin/harness push --person' /tmp/repo",
		"gh pr update-branch 5",
		"gh api -X PUT repos/o/r/pulls/5/update-branch",
	} {
		assert.True(t, bash(policy.Main, line).Deny, line)
	}

	for _, line := range []string{
		"git ls-files | xargs git blame -L1,1",
		"git ls-files '*.go' | xargs grep -n TODO",
		"echo 5 | xargs gh pr view",
	} {
		assert.False(t, bash(policy.Main, line).Deny, "%s: %v", line, bash(policy.Main, line).Reason)
	}
}

// git's own writes are judged like any other: --output and its kin, a patch an agent would apply,
// and the refs the gates compare with.
func TestGitWritesAndRefsAreJudged(t *testing.T) {
	t.Parallel()

	assert.True(t, bash(policy.Reviewer, "git show HEAD:x --output=internal/harness/policy/bash.go").Deny, "a reviewer writes nothing")
	assert.True(t, bash(policy.Coder, "git diff --output=internal/x_test.go").Deny, "the Coder writes no test")
	assert.True(t, bash(policy.Main, "git format-patch -o .cache/harness/evidence HEAD^").Deny, "nobody writes the records")
	assert.True(t, bash(policy.Other, "git apply x.patch").Deny, "a patch names its files inside itself")
	assert.True(t, bash(policy.Other, "git am x.patch").Deny)

	for _, line := range []string{
		"git tag -f origin/main HEAD",
		"git fetch . +HEAD:refs/remotes/origin/main",
		"git update-ref refs/remotes/origin/main HEAD",
	} {
		for _, role := range []policy.Role{policy.Main, policy.Other} {
			assert.True(t, bash(role, line).Deny, "%s: %s", role, line)
		}
	}

	assert.True(t, bash(policy.Other, "git tag v1.2.3").Deny, "tags are a person's, at release")
	assert.False(t, bash(policy.Other, "git tag -l").Deny)
	assert.False(t, bash(policy.Main, "git tag v1.2.3").Deny)
	assert.True(t, bash(policy.Other, "git apply --stat --apply x.patch").Deny, "--apply applies after --stat")
	assert.True(t, bash(policy.Other, "git apply --check --ap x.patch").Deny, "git takes --apply cut short")
	assert.True(t, bash(policy.Other, "git apply --stat --no-stat x.patch").Deny, "--no-stat applies too: an agent applies no patch")

	for _, line := range []string{"git tag --sort=refname v1", "git tag -i v1", "git tag --format=x v1", "git tag --column v1"} {
		assert.True(t, bash(policy.Other, line).Deny, "an agent makes no tag, whatever option comes first: %s", line)
	}

	assert.True(t, bash(policy.Main, "git tag --sort=refname origin/main").Deny, "--sort lists nothing: it makes origin/main")
	assert.False(t, bash(policy.Other, "git tag -l 'v1.*'").Deny)

	for _, line := range []string{"git tag --merged origin/main", "git tag --contains main", "git tag -n5"} {
		for _, role := range []policy.Role{policy.Main, policy.Other} {
			d := bash(role, line)
			assert.False(t, d.Deny, "%s: %s: %v", role, line, d.Reason)
		}
	}

	assert.False(t, bash(policy.Main, "git tag -a v1.2.3 -m 'main release' main").Deny, "a tag of main is not a move of main")
	assert.False(t, bash(policy.Other, "git fetch origin").Deny)
}

// A $ the shell does not expand is text: an agent's awk and sed programs use it, and so do gh's
// alias placeholders and git config's patterns. One the shell expands is still refused.
func TestALiteralDollarIsText(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"awk '{print $1}' docs/a.md",
		"awk -F: '{print $NF}' docs/a.md",
		"sed -n '$p' docs/a.md",
		"sed 's/ *$//' docs/a.md",
	} {
		for _, role := range []policy.Role{policy.Reviewer, policy.Coder} {
			d := bash(role, line)
			assert.False(t, d.Deny, "%s: %s: %v", role, line, d.Reason)
		}
	}

	assert.False(t, bash(policy.Main, `gh alias set epics 'issue list --author="$1" --label=epic'`).Deny)
	assert.False(t, bash(policy.Main, `git config --get-regexp '^remote\..*\.url$'`).Deny)
	assert.True(t, bash(policy.Coder, `S='{print}'; awk "$S" docs/a.md`).Deny, "a program the shell builds is not read")
	assert.True(t, bash(policy.Coder, `S='w x'; sed -n "-e$S" docs/a.md`).Deny, "nor one glued to -e")
	assert.True(t, bash(policy.Coder, `sed "--expression=$(cat s)" docs/a.md`).Deny, "nor one given to --expression=")
}

// BSD's sed (this host's) edits in place with -I as with -i, and takes a lone -i's next word as
// the backup suffix; gawk edits in place with -i inplace.
func TestEveryInPlaceEditIsJudged(t *testing.T) {
	t.Parallel()

	for _, role := range []policy.Role{policy.Reviewer, policy.Coder} {
		assert.True(t, bash(role, "sed -I '' -e 's/a/b/' internal/harness/policy/bash.go").Deny, role)
	}

	d := bash(policy.Coder, "sed -i 's/a/b/' errors.go")
	assert.True(t, d.Deny)
	assert.Contains(t, d.Reason, "backup suffix", "the hint says how to write it on this host")
	assert.False(t, bash(policy.Coder, "sed -i '' 's/a/b/' errors.go").Deny)
	assert.False(t, bash(policy.Coder, "sed -i.bak 's/a/b/' errors.go").Deny)

	assert.True(t, bash(policy.Main, "gawk -i inplace '{print}' .cache/harness/state.json").Deny, "inplace rewrites its operands")
	assert.True(t, bash(policy.Coder, "gawk -i inplace '{print}' docs/a.md").Deny, "an agent loads no extension")
}

// GitHub asks for a person's review of what only a person may change: .github/CODEOWNERS names
// exactly the protected classes of the policy.
func TestCodeOwnersNameTheProtectedClasses(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(filepath.Join("..", "..", "..", ".github", "CODEOWNERS"))
	require.NoError(t, err)

	var owned []string

	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}

		p := strings.TrimPrefix(fields[0], "/")
		if strings.HasSuffix(p, "/") {
			p += "**"
		}

		owned = append(owned, p)
	}

	assert.ElementsMatch(t, policy.ProtectedPatterns(), owned)
}

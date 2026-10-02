package policy_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
)

// The harness's arguments are read as they are written: the smuggling a policy that only refused
// bin/harness push would miss is refused to bin/harness pr too, for the reason it is.
func TestTheHarnessArgumentsAreReadAsWritten(t *testing.T) {
	t.Parallel()

	const (
		person   = "a person's own word"
		writeOut = "write the harness's arguments out"
	)

	for line, reason := range map[string]string{
		"bin/harness pr --person":                       person,
		"bin/harness pr -person=true":                   person,
		"make pr FORCE='1 --person'":                    person,
		"P=--person; bin/harness pr $P":                 writeOut,
		"bin/harness pr $(echo --person)":               writeOut,
		"bin/harness pr --perso[n]":                     writeOut,
		`find . -maxdepth 0 -exec bin/harness pr {} \;`: writeOut,
	} {
		d := bash(policy.Main, line)
		assert.True(t, d.Deny, line)
		assert.Contains(t, d.Reason, reason, line)
	}
}

// A path is judged where it lands: through a symlink on its way or at its end, dangling or not,
// and with the names a refusal guards in any case, since this host's filesystem ignores it.
func TestAPathIsJudgedWhereItLands(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".cache", "harness", "evidence"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "gates", "canaries"), 0o750))

	elsewhere := t.TempDir()
	cache := filepath.Join(elsewhere, "c")
	require.NoError(t, os.Symlink(filepath.Join(root, ".cache"), cache))

	dangling := filepath.Join(elsewhere, "new.json")
	require.NoError(t, os.Symlink(filepath.Join(root, ".cache", "harness", "evidence", "review-x.json"), dangling))

	canaries := filepath.Join(elsewhere, "k")
	require.NoError(t, os.Symlink(filepath.Join(root, "gates", "canaries"), canaries))

	for _, p := range []string{
		filepath.Join(cache, "harness", "evidence", "review-x.json"),
		dangling,
		".CACHE/harness/evidence/review-x.json",
		".cache/Harness/STATE.json",
		".GIT/config",
	} {
		assert.True(t, policy.Write(policy.Main, root, p).Deny, p)
	}

	assert.True(t, policy.Bash(policy.Main, root, root, "echo '{}' > "+cache+"/harness/evidence/review-x.json").Deny)
	assert.True(t, policy.Read(policy.Coder, root, filepath.Join(canaries, "x.patch")).Deny, "a canary through a link")
	assert.True(t, policy.Write(policy.Coder, root, "CLAUDE.MD").Deny, "CLAUDE.md, spelt in another case")
	assert.True(t, policy.Write(policy.Tester, root, "internal/x_TEST.go").Deny, "go builds it: it is code, not a test")
	assert.False(t, policy.Write(policy.Main, root, filepath.Join(elsewhere, "notes.md")).Deny)
}

// gh's command is read past the repository flag gh accepts before it, approving in every form
// pflag reads is refused, and so are API writes that move a branch.
func TestGhIsReadAsGhReadsIt(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"gh pr -R o/r merge 5 --squash",
		"gh pr --repo=o/r merge 5",
		"gh pr -Ro/r merge 5",
		"gh pr --repo o/r review 5 --approve",
		"gh pr review 5 -ab LGTM",
		"gh pr review 5 --approve=true",
		"gh pr --comments merge 5",
		"gh alias set m 'pr -R o/r merge'",
		"gh api -X PATCH repos/o/r/git/refs/heads/main -f sha=abc",
		"gh api -X PUT repos/o/r/contents/README.md -f message=x -f content=eA==",
		"gh api -X POST repos/o/r/branches/main/rename -f new_name=x",
		`gh api graphql -f query='mutation { createCommitOnBranch(input: {}) { commit { oid } } }'`,
		"git --attr-source HEAD push origin HEAD:main",
		"git --frobnicate push origin HEAD:main",
		"git config trailer.x.cmd 'git push origin HEAD:main'",
		"git config core.alternateRefsCommand 'bin/harness push'",
		"git config init.templateDir /tmp/t",
		"git config --rename-section a b",
		"tar -x --to-command='bin/harness push' -f /tmp/a.tgz -C /tmp/out",
		"tar -czf /tmp/x.tgz --checkpoint-action=exec='git push origin HEAD:main' docs",
		"cp -r /tmp/x/.cache .",
	} {
		assert.True(t, bash(policy.Main, line).Deny, line)
	}

	for _, line := range []string{
		"gh pr -R o/r view 5",
		"gh pr review 5 --comment -b 'looks good'",
		"gh pr review 5 -c -b x",
		"gh api -X GET repos/o/r/pulls",
		"gh api repos/o/r/contents/README.md",
		"git --no-pager log -1",
		"git --attr-source=HEAD log -1",
		"git -C /tmp/x status",
		"git config user.email a@example.com",
		"git config core.autocrlf false",
		"unzip -p /tmp/x.mcpb manifest.json",
		"tar -xOf /tmp/r.tgz docs/a.md",
		"tar -tf /tmp/r.tgz",
		"mv /tmp/notes.md .",
	} {
		assert.False(t, bash(policy.Main, line).Deny, "%s: %v", line, bash(policy.Main, line).Reason)
	}

	assert.False(t, bash(policy.Other, "gh pr -R o/r view 5").Deny, "an agent reads a pull request of another repository")
}

package policy_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
)

// A command git, gh or the shell runs later, set by configuration or the environment, is judged as
// the command line it is: an alias does not carry a push the policy refuses. Configuration the
// policy cannot read (a file it loads, the editor, a file written by hand) is refused.
func TestConfigurationThatRunsCommandsIsJudgedAsTheCommand(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"bin/harness push",
		`git config alias.p '!bin/harness push --person'`,
		"git config alias.m 'push origin HEAD:main'",
		`git config --global alias.p '!bin/harness push'`,
		`git config set --global alias.p '!bin/harness push'`,
		"git config --add alias.m 'push origin HEAD:main'",
		"git config core.pager 'bin/harness push --person'",
		"git config diff.x.textconv 'bin/harness push'",
		"git config --add include.path /tmp/x.cfg",
		"git config includeIf.gitdir:~/x/.path /tmp/x.cfg",
		"git config -e",
		"git config --global --edit",
		"git config edit",
		"git config --rename-section x alias",
		"gh alias set --shell p 'bin/harness push --person'",
		"gh alias set m 'pr merge 1'",
		"gh alias set p '!bin/harness push'",
		"gh alias set p -",
		"gh alias import aliases.yml",
		"gh config set pager 'bin/harness push --person'",
		"gh extension install owner/gh-x",
		"GIT_PAGER='bin/harness push --person' git log",
		"env EDITOR='bin/harness push' git commit",
		"HOME=/tmp/h git p",
		"XDG_CONFIG_HOME=/tmp/c git p",
		"GH_CONFIG_DIR=/tmp/g gh p",
		"echo '[alias] p = !x' >> ~/.gitconfig",
		"cp /tmp/x ~/.gitconfig-work",
		"tee ~/.config/gh/config.yml",
		`awk 'BEGIN { system("bin/harness push --person") }'`,
		"sed '1e bin/harness push' in.txt",
	} {
		for _, role := range []policy.Role{policy.Main, policy.Other} {
			assert.True(t, bash(role, line).Deny, "%s: %s", role, line)
		}
	}

	for _, line := range []string{
		"git config --get alias.p",
		"git config -l",
		"git config alias.p",
		"git config alias.st status",
		"git config --global alias.lg 'log --oneline'",
		"git config core.pager 'less -FRX'",
		"git config branch.feat/x.description 'push origin main later'",
		"gh alias set co 'pr checkout'",
		"gh config set editor vim",
		"gh extension list",
		"GIT_PAGER=cat git log",
		"GIT_EDITOR=true git rebase --continue",
		"EDITOR=vim git commit",
		"grep -rn -- --person cmd/harness",
		`echo "only a person passes --person"`,
		"rg -e --person .",
	} {
		assert.False(t, bash(policy.Main, line).Deny, "%s: %v", line, bash(policy.Main, line).Reason)
	}

	assert.True(t, bash(policy.Coder, "git config alias.st status").Deny, "an agent's configuration is the repository's")
	assert.True(t, policy.Write(policy.Main, root, "~/.gitconfig").Deny, "the editor is refused git's configuration too")
}

// The files a command writes besides its output are judged like a redirection: whatever form its
// options take, it may not write the harness's records.
func TestEveryWayACommandNamesTheFileItWritesIsJudged(t *testing.T) {
	t.Parallel()

	const ev = ".cache/harness/evidence/abc.json"

	for _, line := range []string{
		"find . -maxdepth 0 -fprintf " + ev + " '{}'",
		"find . -maxdepth 0 -fprint .cache/harness/state.json",
		"find . -maxdepth 0 -fls " + ev,
		"sed -n 'w " + ev + "' in.txt",
		"sed -n '/x/w " + ev + "' in.txt",
		"sed 's/a/b/w " + ev + "' in.txt",
		"sed -e p -e 'W " + ev + "' in.txt",
		"sed -i '' s/a/b/ " + ev,
		"sed -i -e s/a/b/ " + ev,
		"sed -ni.bak p " + ev,
		"uniq in.txt " + ev,
		"uniq -c -f 1 in.txt " + ev,
		"xxd -r -p hex.txt " + ev,
		`yq -i '.verdict = "pass"' ` + ev,
		`yq -iP '.verdict = "pass"' ` + ev,
		`awk 'BEGIN { print "{}" > "` + ev + `" }'`,
		`awk '{ printf "%s", $0 >> "` + ev + `" }' in.txt`,
		"curl -sSo " + ev + " http://localhost/x",
		"curl -o" + ev + " http://localhost/x",
		"curl --output=" + ev + " http://localhost/x",
		"curl -D " + ev + " http://localhost/x",
		"curl -sSLO --output-dir .cache/harness/evidence http://localhost/abc.json",
		"wget -qO " + ev + " http://localhost/x",
		"wget --output-document=" + ev + " http://localhost/x",
		"wget -P .cache/harness/evidence http://localhost/abc.json",
		"cd .cache/harness/evidence && wget -q http://localhost/abc.json",
		"cd .cache/harness/evidence && curl -O http://localhost/abc.json",
		"sort -uo " + ev + " in.txt",
		"sort --output " + ev + " in.txt",
		"mv -t .cache/harness/evidence /tmp/abc.json",
		"cp --target-directory=.cache/harness/evidence /tmp/abc.json",
		"tar -xzf /tmp/e.tgz --directory=.cache/harness",
		"unzip -qd .cache/harness /tmp/e.zip",
	} {
		for _, role := range []policy.Role{policy.Main, policy.Other} {
			assert.True(t, bash(role, line).Deny, "%s: %s", role, line)
		}
	}

	for _, line := range []string{
		"rm -rf .cache",
		"chmod -R u+w .",
		"sed -i '' s/a/b/ docs/a.md",
		"sed -n '/ w foo/p' internal/x.go",
		"sed -e 'a\\' -e 'text' docs/a.md",
		`awk '$1 > "m" { print }' in.txt`,
		"curl -sSo /tmp/x http://localhost/x",
		"wget -qO- http://localhost/x",
		"sort -o /tmp/x in.txt",
		"sort -k1,1 -t: in.txt",
		"uniq -c in.txt",
		"xxd internal/x.go",
		"find . -name '*.go' -fprint /tmp/list",
		"tar -xzf /tmp/e.tgz -C /tmp/out",
	} {
		assert.False(t, bash(policy.Main, line).Deny, "%s: %v", line, bash(policy.Main, line).Reason)
	}

	assert.False(t, bash(policy.Tester, "sed -n '/ w foo/p' internal/x_test.go").Deny, "a w inside an address writes nothing")
	assert.True(t, bash(policy.Tester, `awk '{ print > "m" }' in.txt`).Deny, "awk's print > writes the file m")
	assert.True(t, bash(policy.Coder, "sed '1e date' in.txt").Deny, "an agent's sed runs no command")
}

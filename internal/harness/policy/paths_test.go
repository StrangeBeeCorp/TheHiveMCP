package policy_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
)

const root = "/r/thehivemcp"

// repoFile is a file of this repository, read from the package's directory.
func repoFile(t *testing.T, rel string) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "..", rel)) // #nosec G304 -- a file of this repository the test names
	require.NoError(t, err)

	return data
}

// A command line is refused only for naming an environment file: what it does is Claude Code's
// to judge, and what it changed the Stop hook's. The commands the shell parser refused for
// spellings it could not read run.
func TestACommandIsRefusedOnlyForAnEnvironmentFile(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"cat .env", "source .env", "grep KEY .env.local", "cp .env /tmp/x", "git show HEAD:.env",
		"docker run --env-file=.env x", "cat $HOME/app/.env.prod",
	} {
		d := policy.Bash(line)
		assert.True(t, d.Deny, line)
		assert.Contains(t, d.Reason, ".env.template", "the denial names where the keys are")
	}

	for _, line := range []string{
		"cat .env.template", "sed -i '' 's/a/b/' notes.md", `awk '{print $1 > "out.txt"}' in.txt`, "tar -xf /tmp/e.tar -C /tmp/x",
		"git lg", "python3 -c 'print(1)'", "make check-fast",
	} {
		assert.False(t, policy.Bash(line).Deny, line)
	}
}

// What only a person does is refused to Claude Code itself, in the plain forms Claude writes, by
// the project's permission rules; make pr pushes from inside the harness, not as a command line.
func TestClaudeCodeRefusesWhatOnlyAPersonDoes(t *testing.T) {
	t.Parallel()

	var settings struct {
		Permissions struct {
			Deny []string `json:"deny"`
		} `json:"permissions"`
	}

	require.NoError(t, json.Unmarshal(repoFile(t, ".claude/settings.json"), &settings))

	refused := func(command string) bool {
		return slices.ContainsFunc(settings.Permissions.Deny, func(rule string) bool { return denies(rule, command) })
	}

	for _, command := range []string{
		"git push", "git push origin HEAD:main", "git send-pack origin main", "gh pr merge 12 --squash", "gh pr review --approve",
		"gh pr review 5 --approve", "gh pr update-branch 3", "bin/harness push", "./bin/harness push --force", "bin/harness pr --person",
	} {
		assert.True(t, refused(command), command)
	}

	for _, command := range []string{
		"make pr", "make pr FORCE=1", "git status", "git commit -m x", "gh pr view 3", "gh pr review 5 --comment -b ok",
		"bin/harness pr", "bin/harness review-effort",
	} {
		assert.False(t, refused(command), command)
	}
}

// denies reports whether a Bash deny rule matches a command as Claude Code documents it: * stands
// for any text, and a trailing " *" that is the rule's only wildcard also matches the bare command.
func denies(rule, command string) bool {
	pattern, ok := strings.CutPrefix(rule, "Bash(")
	if !ok {
		return false
	}

	pattern = strings.TrimSuffix(pattern, ")")
	if strings.Count(pattern, "*") == 1 && strings.HasSuffix(pattern, " *") && command == strings.TrimSuffix(pattern, " *") {
		return true
	}

	return regexp.MustCompile("^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, ".*") + "$").MatchString(command)
}

// GitHub asks for a person's review of what only a person may change: .github/CODEOWNERS names
// exactly the protected classes of the policy.
func TestCodeOwnersNameTheProtectedClasses(t *testing.T) {
	t.Parallel()

	var owned []string

	for line := range strings.SplitSeq(string(repoFile(t, ".github/CODEOWNERS")), "\n") {
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

// A path is judged where it lands: through a symlink on its way or at its end, dangling or not,
// and with the names a refusal guards in any case, since this host's filesystem ignores it.
func TestAPathIsJudgedWhereItLands(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".cache", "harness", "evidence"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "gates", "canaries"), 0o750))

	elsewhere := t.TempDir()
	cache := filepath.Join(elsewhere, "c")
	require.NoError(t, os.Symlink(filepath.Join(dir, ".cache"), cache))

	dangling := filepath.Join(elsewhere, "new.json")
	require.NoError(t, os.Symlink(filepath.Join(dir, ".cache", "harness", "evidence", "review-x.json"), dangling))

	canaries := filepath.Join(elsewhere, "k")
	require.NoError(t, os.Symlink(filepath.Join(dir, "gates", "canaries"), canaries))

	for _, p := range []string{
		filepath.Join(cache, "harness", "evidence", "review-x.json"),
		dangling,
		".CACHE/harness/evidence/review-x.json",
		".cache/Harness/STATE.json",
		".GIT/config",
	} {
		assert.True(t, policy.Write(policy.Main, dir, p).Deny, p)
	}

	assert.True(t, policy.Read(policy.Coder, dir, filepath.Join(canaries, "x.patch")).Deny, "a canary through a link")
	assert.True(t, policy.Write(policy.Coder, dir, "CLAUDE.MD").Deny, "CLAUDE.md, spelt in another case")
	assert.True(t, policy.Write(policy.Tester, dir, "internal/x_TEST.go").Deny, "go builds it: it is code, not a test")
	assert.False(t, policy.Write(policy.Main, dir, filepath.Join(elsewhere, "notes.md")).Deny)
}

func TestRolesComeFromAgentTypes(t *testing.T) {
	t.Parallel()

	for agentType, want := range map[string]policy.Role{
		"":                           policy.Main,
		"tester":                     policy.Tester,
		"coder":                      policy.Coder,
		"sb-personal:rv-sonnet-high": policy.Reviewer,
		"reviewer-trust":             policy.Reviewer,
		"Explore":                    policy.Reviewer,
		"general-purpose":            policy.Other,
	} {
		assert.Equal(t, want, policy.RoleOf(agentType), agentType)
	}
}

func TestTheCoderNeverEditsATest(t *testing.T) {
	t.Parallel()

	for _, p := range []string{
		"internal/tools/search/search_test.go",
		"internal/tools/search/testdata/page.json",
		"internal/testutils/container.go",
		".claude/sealed/DL-1/search_test.go",
		root + "/internal/tools/manage/manage_test.go",
	} {
		d := policy.Write(policy.Coder, root, p)
		assert.True(t, d.Deny, p)
	}

	assert.False(t, policy.Write(policy.Coder, root, "internal/tools/search/handler.go").Deny)
	assert.False(t, policy.Write(policy.Coder, root, "CHANGELOG.md").Deny)
}

func TestTheTesterWritesOnlyTests(t *testing.T) {
	t.Parallel()

	for _, p := range []string{"internal/tools/search/handler.go", "README.md", "go.mod"} {
		d := policy.Write(policy.Tester, root, p)
		assert.True(t, d.Deny, p)
		assert.Contains(t, d.Reason, "questions", "the Tester is told how to ask for the change")
	}

	for _, p := range []string{"internal/tools/search/search_test.go", "internal/testutils/x.go", ".claude/sealed/DL-1/x_test.go", policy.BlockedFile} {
		assert.False(t, policy.Write(policy.Tester, root, p).Deny, p)
	}

	assert.True(t, policy.Write(policy.Tester, root, "internal/harness/policy/paths_test.go").Deny, "the harness's own tests are harness policy")
}

func TestProtectedClassesAreTheMainSessions(t *testing.T) {
	t.Parallel()

	for _, p := range []string{
		"gates/limits.yaml", ".claude/settings.json", ".claude/agents/coder.md", "spec/items/SIL-001.yaml",
		"baselines/surface/tools.json", "internal/permissions/embedded/default_permissions.yaml", "Makefile", "scripts/harness.mk",
		".github/workflows/ci.yml", "cmd/harness/main.go",
	} {
		d := policy.Write(policy.Other, root, p)
		assert.True(t, d.Deny, p)
		assert.False(t, policy.Write(policy.Main, root, p).Deny, p)
	}

	assert.True(t, policy.Write(policy.Main, root, ".git/config").Deny, "nobody writes inside .git")

	for _, p := range []string{"~/.gitconfig", "~/.config/git/config", "~/.config/gh/hosts.yml"} {
		assert.True(t, policy.Write(policy.Main, root, p).Deny, "%s is git's or gh's configuration, a person's", p)
	}

	assert.True(t, policy.Write(policy.Reviewer, root, "notes.md").Deny)
	assert.True(t, policy.Write(policy.Coder, root, "/Users/example/x.txt").Deny)
	assert.False(t, policy.Write(policy.Coder, root, "/tmp/x.txt").Deny)
}

func TestReadsKeepSecretsCanariesAndSealedTestsAway(t *testing.T) {
	t.Parallel()

	for _, role := range []policy.Role{policy.Main, policy.Coder, policy.Reviewer} {
		assert.True(t, policy.Read(role, root, ".env").Deny)
		assert.True(t, policy.Read(role, root, "/elsewhere/.env.local").Deny)
		assert.False(t, policy.Read(role, root, ".env.template").Deny)
	}

	assert.True(t, policy.Read(policy.Coder, root, "gates/canaries/SIL-001.patch").Deny)
	assert.True(t, policy.Read(policy.Tester, root, "gates/canaries/SIL-001.patch").Deny)
	assert.False(t, policy.Read(policy.Main, root, "gates/canaries/SIL-001.patch").Deny)
	assert.True(t, policy.Read(policy.Coder, root, ".claude/sealed/DL-1/x_test.go").Deny)
	assert.True(t, policy.Read(policy.Coder, root, ".cache/harness/agents/ab12/.claude/sealed/DL-1/x_test.go").Deny,
		"the Stop hook's copy of a sealed test is as sealed as it")
	assert.False(t, policy.Read(policy.Tester, root, ".claude/sealed/DL-1/x_test.go").Deny)
}

func TestTheHarnessRecordsAreNobodysToWrite(t *testing.T) {
	t.Parallel()

	for _, role := range []policy.Role{policy.Main, policy.Coder, policy.Tester, policy.Other} {
		assert.True(t, policy.Write(role, root, ".cache/harness/evidence/1111111.json").Deny, "%s writes the evidence", role)
		assert.True(t, policy.Write(role, root, ".cache/harness/state.json").Deny, "%s writes the hook state", role)
	}

	assert.False(t, policy.Write(policy.Coder, root, policy.BlockedFile).Deny, "BLOCKED.md stays the agent's to write")
}

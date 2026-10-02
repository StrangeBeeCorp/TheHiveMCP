package policy_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
)

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
	assert.False(t, policy.Read(policy.Tester, root, ".claude/sealed/DL-1/x_test.go").Deny)
}

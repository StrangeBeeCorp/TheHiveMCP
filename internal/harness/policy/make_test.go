package policy_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
)

// A value the Makefile expands into a recipe is judged wherever make gets it: its command line, a
// prefix assignment, env, an export, eval, or a variable the policy cannot read.
func TestMakeVariablesAreJudgedWhereverMakeReadsThem(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"ID='x; git push public main' make gate",
		"env ID='x; git push public main' make gate",
		"export ID='x; git push public main'; make gate",
		"ID=$(printf 'x;git push public main') make gate",
		`eval "export ID='x; git push'"; make gate`,
		"read ID < /tmp/id; export ID; make gate",
		`v=ID; export "$v"; make gate`,
		"set -a; ID=x; make gate",
		"set -o allexport; make gate",
	} {
		for _, role := range []policy.Role{policy.Main, policy.Coder, policy.Other} {
			assert.True(t, bash(role, line).Deny, "%s: %s", role, line)
		}
	}

	for _, line := range []string{
		"THEHIVE_TEST_IMAGE=strangebee/thehive:5.5.2 make test-integration > .cache/tmp/it.log 2>&1",
		"export COVERAGE=1; make test",
		"set -e; make test",
	} {
		assert.False(t, bash(policy.Coder, line).Deny, "%s: %v", line, bash(policy.Coder, line).Reason)
	}
}

// make runs this repository's Makefile as it is: no other makefile, directory, evaluated text or
// environment override, however the option is spelled.
func TestMakeRunsTheRepositorysMakefileAsItIs(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"make -f /tmp/evil.mk x", "make -f/tmp/evil.mk x", "make --file=/tmp/evil.mk x", "make --makefile=/tmp/evil.mk x",
		"make --fi /tmp/evil.mk x", "make -sf /tmp/evil.mk x", "make -C /tmp x", "make -C/tmp x", "make --directory=/tmp x",
		"make --dir=/tmp x", "make -e test", "make -se test", "make --eval='x: ; git push' x", "make -E 'x: ; git push' x",
		"make SHELL=/tmp/x.sh test", "make .SHELLFLAGS=-c test", "make MAKE=/tmp/x build-all",
	} {
		for _, role := range []policy.Role{policy.Coder, policy.Other} {
			assert.True(t, bash(role, line).Deny, "%s: %s", role, line)
		}
	}

	for _, line := range []string{"make -j4 test", "make -k test", "make --no-print-directory test", "make -s check-fast"} {
		assert.False(t, bash(policy.Coder, line).Deny, "%s: %v", line, bash(policy.Coder, line).Reason)
	}
}

// An agent passes only the variables make help documents; the others the Makefile sets (its pinned
// images among them) are not an agent's to change.
func TestAnAgentPassesOnlyTheDocumentedMakeVariables(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	makefile := "GO_IMAGE := golang:1.26.8\nTHEHIVE_TEST_URL ?= http://localhost:9000\n" +
		"test: ## Unit tests ##@ make test [COVERAGE=1] [RUN=<regexp>]\n\tgo test\n" +
		"test-integration: ## Integration ##@ make test-integration [THEHIVE_TEST_URL=..]\n\ttrue\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "Makefile"), []byte(makefile), 0o600))

	in := func(role policy.Role, line string) policy.Decision { return policy.Bash(role, root, root, line) }

	assert.True(t, in(policy.Coder, "make test GO_IMAGE=attacker/image").Deny)
	assert.True(t, in(policy.Other, "make test GO_IMAGE:=attacker/image").Deny)
	assert.False(t, in(policy.Coder, "make test RUN=TestSearch COVERAGE=1").Deny)
	assert.False(t, in(policy.Coder, "make test-integration THEHIVE_TEST_URL=http://127.0.0.1:9000").Deny)
	assert.False(t, in(policy.Coder, "make test UNKNOWN=1").Deny, "a variable the Makefile does not know changes nothing")
	assert.False(t, in(policy.Main, "make test GO_IMAGE=golang:1.27").Deny, "the main session may")
}

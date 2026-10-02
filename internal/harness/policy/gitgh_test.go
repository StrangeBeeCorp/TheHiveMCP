package policy_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
)

var everyRole = []policy.Role{policy.Main, policy.Coder, policy.Other}

// git reads its options as parse-options does: short ones cluster and long ones may be cut short,
// so the hooks are skipped however --no-verify or -n is spelled.
func TestTheHooksCannotBeSkippedHoweverTheOptionIsSpelled(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"git commit -nm x", "git commit -anm x", "git commit -vn -m x", "git commit --no-veri -m x", "git commit --no-verif -m x",
		"git am -n x.patch", "git rebase --no-verify origin/main", "git merge --no-verify feat/x",
		"git config core.hookspath /dev/null", "git config --local CORE.HOOKSPATH x", "git config -- core.hooksPath /dev/null",
	} {
		for _, role := range everyRole {
			assert.True(t, bash(role, line).Deny, "%s: %s", role, line)
		}
	}

	for _, line := range []string{"git commit -mn", "git commit -m 'no -n here'", "git merge -n feat/x", "git commit -m x -- --no-verify"} {
		assert.False(t, bash(policy.Main, line).Deny, "%s: %v", line, bash(policy.Main, line).Reason)
	}

	assert.True(t, bash(policy.Other, "git commit -pn -m x").Deny, "commit's -p is --patch: the n is an option")
	assert.False(t, bash(policy.Main, "git am -p1n x.patch").Deny, "am's -p takes 1n as its value")
	assert.True(t, bash(policy.Other, "git fetch --up=x origin").Deny, "a prefix of --upload-pack, which git accepts")
}

// What git runs for its caller is judged as the command line it is.
func TestWhatGitRunsIsJudged(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"git rebase -x 'git push public main' origin/main",
		"git rebase --exec='git push' origin/main",
		"git rebase --exe 'git push' origin/main",
		"git rebase -ix 'git push' origin/main",
		"git submodule foreach 'git push public main'",
		"git submodule foreach --recursive git push",
		"git bisect run git push",
		`git grep -O'sh -c "git push"' TODO`,
		"git difftool --extcmd='git push' HEAD~1",
	} {
		for _, role := range everyRole {
			assert.True(t, bash(role, line).Deny, "%s: %s", role, line)
		}
	}

	for _, line := range []string{"git fetch --upload-pack='sh x' origin", "git clone -u 'sh x' https://example.com/r.git", "git ls-remote --exec=x origin"} {
		assert.True(t, bash(policy.Other, line).Deny, line)
	}

	assert.False(t, bash(policy.Main, "git rebase -x 'make test' origin/main").Deny)
	assert.False(t, bash(policy.Other, "git rebase --exec 'make test' origin/main").Deny, "a rebase's --exec is not a transport")
}

// The variables that make a program run another command, or load code, are not an agent's to set
// beyond a no-op.
func TestAnAgentDoesNotSetWhatRunsCode(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		`GIT_EXTERNAL_DIFF='sh -c "git push"' git diff`,
		"GIT_PAGER='git push' git log",
		"PATH=.cache/tmp:$PATH make test",
		"LD_PRELOAD=/tmp/x.so git status",
		"DYLD_INSERT_LIBRARIES=/tmp/x.dylib git status",
		"env BASH_ENV=/tmp/x.sh bash -c true",
		"export GIT_SSH_COMMAND='sh /tmp/x'",
	} {
		for _, role := range []policy.Role{policy.Coder, policy.Other} {
			assert.True(t, bash(role, line).Deny, "%s: %s", role, line)
		}
	}

	for _, line := range []string{"GIT_PAGER=cat git log", "GIT_EDITOR=true git status", "PAGER= git diff"} {
		assert.False(t, bash(policy.Coder, line).Deny, "%s: %v", line, bash(policy.Coder, line).Reason)
	}
}

// gh api is read as gh reads it: -XPUT, an endpoint after a boolean option, fields that make a
// POST, and GraphQL, whose mutations merge and review as REST does.
func TestGhAPIIsReadAsGhReadsIt(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"gh api -XPUT repos/o/r/pulls/198/merge",
		"gh api -iXPUT repos/o/r/pulls/198/merge",
		"gh api --method=PUT repos/o/r/pulls/198/merge",
		"gh api repos/o/r/issues -F title=x",
		"gh api graphql -f query='query{viewer{login}}'",
	} {
		assert.True(t, bash(policy.Coder, line).Deny, "agents only read: %s", line)
	}

	for _, line := range []string{
		"gh api -X PUT -i repos/o/r/pulls/198/merge",
		"gh api --paginate -X POST repos/o/r/statuses/abc -f state=success",
		`gh api -X POST "$EP" -f state=success`,
		`gh api graphql -f query='mutation{mergePullRequest(input:{pullRequestId:"x"}){clientMutationId}}'`,
		`gh api graphql -f query='mutation{addPullRequestReview(input:{pullRequestId:"x",event:APPROVE}){clientMutationId}}'`,
		"gh api graphql -F query=@merge.graphql",
		"gh api graphql --input body.json",
		`gh api graphql -f query="$Q"`,
		`gh api graphql -f query="mutation{$(cat m)}"`,
	} {
		assert.True(t, bash(policy.Main, line).Deny, "Main: %s", line)
	}

	for _, line := range []string{
		"gh api -XGET repos/o/r/pulls",
		"gh api --paginate repos/o/r/pulls/198/comments",
		"gh api -X GET repos/o/r/pulls -f state=open",
		"gh api repos/o/r/pulls/198/merge",
	} {
		assert.False(t, bash(policy.Coder, line).Deny, "%s: %v", line, bash(policy.Coder, line).Reason)
	}

	for _, line := range []string{
		"gh api graphql -f query='query{viewer{login}}'",
		`gh api graphql -f query='query($o:String!){repository(owner:$o,name:"x"){id}}' -f o=x`,
		"gh api -X PATCH repos/o/r/pulls/198 -f title=x",
	} {
		assert.False(t, bash(policy.Main, line).Deny, "%s: %v", line, bash(policy.Main, line).Reason)
	}
}

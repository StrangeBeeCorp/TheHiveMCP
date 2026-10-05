package policy

import "strings"

// The command words the rules name in more than one place.
const (
	cmdGit       = "git"
	cmdMake      = "make"
	cmdSed       = "sed"
	cmdSort      = "sort"
	cmdUnzip     = "unzip"
	cmdPrintf    = "printf"
	cmdCat       = "cat"
	cmdEcho      = "echo"
	cmdHead      = "head"
	cmdTail      = "tail"
	keyEditor    = "editor"
	keyPager     = "pager"
	cmdAwk       = "awk"
	cmdTar       = "tar"
	cmdInstall   = "install"
	subConfig    = "config"
	subEdit      = "edit"
	subSet       = "set"
	optFile      = "--file"
	optGet       = "--get"
	optContains  = "--contains"
	optMerged    = "--merged"
	optHelp      = "--help"
	subFetch     = "fetch"
	subPull      = "pull"
	subTag       = "tag"
	subUpdateRef = "update-ref"
	subModule    = "submodule"
	keyAlias     = "alias"
	keyCmd       = "cmd"
	keyBrowser   = "browser"
	cmdTrue      = "true"
	subGrep      = "grep"
	subPush      = "push"
	subDiff      = "diff"
	subList      = "list"
	subHelp      = "help"
	subVersion   = "version"
	subStatus    = "status"
	subShow      = "show"
	noNetwork    = "no network from the shell"
	mainBranch   = "main"
	hintFmt      = "make fmt"
	hintLint     = "make lint"
)

// gitCommands are git's own commands (git 2.55, git --list-cmds=main). Any other word is an alias
// or a git-<name> program on PATH: it runs with words the policy reads as another command's.
var gitCommands = strings.Fields(`add am annotate apply archimport archive backfill bisect blame branch
bugreport bundle cat-file check-attr check-ignore check-mailmap check-ref-format checkout checkout-index
cherry cherry-pick citool clean clone column commit commit-graph commit-tree config count-objects credential
describe diagnose diff diff-files diff-index diff-pairs diff-tree difftool fast-export fast-import fetch
fetch-pack filter-branch fmt-merge-msg for-each-ref for-each-repo format-patch format-rev fsck fsck-objects gc
get-tar-commit-id grep gui hash-object help history hook http-push imap-send index-pack init init-db
instaweb interpret-trailers last-modified log ls-files ls-remote ls-tree mailinfo mailsplit maintenance merge
merge-base merge-file merge-index merge-one-file merge-tree mergetool mktag mktree multi-pack-index mv
name-rev notes pack-objects pack-redundant pack-refs patch-id prune prune-packed pull push range-diff
read-tree rebase receive-pack reflog refs remote repack replace replay repo request-pull rerere reset
restore rev-list rev-parse revert rm send-email send-pack shortlog show show-branch show-index show-ref
sparse-checkout stage stash status stripspace submodule subtree switch symbolic-ref tag unpack-file
unpack-objects update-index update-ref update-server-info upload-archive upload-pack var verify-commit
verify-pack verify-tag version whatchanged worktree write-tree`)

// ghCommands are gh's command groups (gh 2.102). Any other word is an alias or an extension.
var ghCommands = strings.Fields(`agent-task alias api attestation auth browse cache codespace completion
config copilot discussion ext extension extensions gist gpg-key help issue label licenses org pr preview
project release repo ruleset run search secret skill ssh-key status variable version workflow`)

func isHarness(name string) bool {
	return name == "bin/harness" || name == "./bin/harness" || strings.HasSuffix(name, "/bin/harness")
}

// harnessWord reports whether word runs the harness: bin/harness by any path, or harness itself.
func harnessWord(word string) bool {
	return isHarness(word) || word == "harness"
}

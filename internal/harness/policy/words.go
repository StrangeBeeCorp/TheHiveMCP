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

func isHarness(name string) bool {
	return name == "bin/harness" || name == "./bin/harness" || strings.HasSuffix(name, "/bin/harness")
}

// harnessWord reports whether word runs the harness: bin/harness by any path, or harness itself.
func harnessWord(word string) bool {
	return isHarness(word) || word == "harness"
}

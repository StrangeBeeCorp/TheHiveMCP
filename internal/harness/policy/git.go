package policy

import (
	"slices"
	"strings"
)

// gitReads are the git subcommands that only read; they are all the duel's agents may run.
var gitReads = []string{
	subStatus, subDiff, "log", subShow, "blame", "grep", "ls-files", "ls-tree", "rev-parse", "rev-list",
	"merge-base", "describe", "cat-file", "shortlog", "whatchanged", "name-rev", "for-each-ref", subHelp,
	subVersion, "check-ignore", "range-diff",
}

// git judges a git command: its own options first, then the subcommand.
func (j *judge) git(args []string) Decision {
	i := 0

	for i < len(args) && strings.HasPrefix(args[i], "-") {
		a := args[i]

		switch {
		case a == "-c" || a == "--config-env" || strings.HasPrefix(a, "--config-env=") || strings.HasPrefix(a, "--exec-path="):
			return deny("per-command git configuration is not allowed: the repository's configuration is policy")
		case slices.Contains([]string{"-C", "--git-dir", "--work-tree", "--namespace", "--super-prefix"}, a):
			if j.role.Agent() && a != "-C" {
				return deny("git runs on this checkout as it is configured")
			}

			i += 2
		case strings.HasPrefix(a, "--git-dir=") || strings.HasPrefix(a, "--work-tree=") || strings.HasPrefix(a, "--namespace="):
			if j.role.Agent() {
				return deny("git runs on this checkout as it is configured")
			}

			i++
		default:
			i++
		}
	}

	if i >= len(args) {
		return Allow
	}

	return j.gitSub(args[i], args[i+1:])
}

func (j *judge) gitSub(sub string, rest []string) Decision {
	has := func(flag string) bool { return slices.Contains(rest, flag) }

	if slices.ContainsFunc(rest, func(a string) bool { return strings.Contains(a, "core.hooksPath") || a == "--no-verify" }) ||
		(sub == "commit" && has("-n")) {
		return deny("the git hooks are gates: commit without bypassing them, and fix what they report")
	}

	if sub == subPush || sub == "send-pack" {
		return deny("push with make push (bin/harness push): it pushes this branch to origin, " +
			"never main and never the public mirror")
	}

	if movesMain(sub, rest) {
		return deny("main moves only when a person merges a pull request on GitHub")
	}

	switch {
	case slices.Contains(gitReads, sub) || readOnlyGit(sub, rest):
		return Allow
	case j.role.Duel():
		return deny("the %s reads git (status, diff, log, show…); the session that runs the duel commits", j.role)
	case j.role.ReadOnly():
		return deny("the %s does not change the repository", j.role)
	}

	if !j.role.Agent() {
		return Allow
	}

	return j.gitAgent(sub, rest)
}

// gitAgent holds for the agents that may commit: history and configuration are not theirs.
func (j *judge) gitAgent(sub string, rest []string) Decision {
	switch sub {
	case "update-ref", "replace", "filter-branch", "filter-repo", "fast-import", "pack-refs", "symbolic-ref":
		return deny("refs and history are the record of the work: they are not rewritten by hand")
	case "reflog":
		if len(rest) > 0 && rest[0] != subShow {
			return deny("refs and history are the record of the work: they are not rewritten by hand")
		}
	case "notes":
		if len(rest) > 0 && !slices.Contains([]string{subList, subShow}, rest[0]) {
			return deny("notes are written by the harness only")
		}
	case "config":
		if !slices.ContainsFunc(rest, func(a string) bool {
			return slices.Contains([]string{"--get", "--get-all", "--get-regexp", "--" + subList, "-l"}, a)
		}) {
			return deny("git configuration is set by make dev-setup and does not change during a task")
		}
	case "remote":
		if len(rest) > 0 && !slices.Contains([]string{"-v", subShow, "get-url"}, rest[0]) {
			return deny("remotes are configured by people")
		}
	}

	return Allow
}

// readOnlyGit reports whether a subcommand outside gitReads only reads with these arguments.
func readOnlyGit(sub string, rest []string) bool {
	switch sub {
	case "branch":
		return len(rest) == 0 || slices.ContainsFunc(rest, func(a string) bool {
			return slices.Contains([]string{"--show-current", "--" + subList, "-l", "-a", "-r", "-v", "-vv", "--contains", "--merged"}, a)
		})
	case "remote":
		return len(rest) == 0 || rest[0] == "-v" || rest[0] == subShow || rest[0] == "get-url"
	case "worktree":
		return len(rest) > 0 && rest[0] == subList
	}

	return false
}

// movesMain reports whether a git command would rewrite the local main by hand. Main itself
// changes on GitHub only: no one pushes it from here (bin/harness push refuses).
func movesMain(sub string, rest []string) bool {
	isMain := func(a string) bool { return a == mainBranch || strings.HasSuffix(a, "/"+mainBranch) }

	switch sub {
	case "branch":
		var positional []string

		for k := 0; k < len(rest); k++ {
			switch {
			case slices.Contains([]string{"--contains", "--no-contains", "--merged", "--no-merged", "--points-at",
				"--sort", "--format", "-u", "--set-upstream-to"}, rest[k]):
				k++
			case !strings.HasPrefix(rest[k], "-"):
				positional = append(positional, rest[k])
			}
		}

		listing := slices.Contains(rest, "--"+subList) || slices.Contains(rest, "-l")

		return !listing && slices.ContainsFunc(positional, isMain)
	case "checkout", "switch", "worktree":
		for k := 0; k+1 < len(rest); k++ {
			if slices.Contains([]string{"-b", "-B", "-c", "-C", "--orphan", "--force-create"}, rest[k]) && isMain(rest[k+1]) {
				return true
			}
		}
	case "update-ref":
		return slices.ContainsFunc(rest, func(a string) bool { return a == "refs/heads/"+mainBranch || a == mainBranch })
	}

	return false
}

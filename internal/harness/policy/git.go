package policy

import (
	"slices"
	"strings"
)

// gitReads are the git subcommands that only read; they are all the duel's agents may run.
var gitReads = []string{
	subStatus, subDiff, "log", subShow, "blame", subGrep, "ls-files", "ls-tree", "rev-parse", "rev-list",
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
	if skipsHooks(sub, rest) {
		return deny("the git hooks are gates: commit without bypassing them, and fix what they report")
	}

	if d := j.gitRuns(sub, rest); d.Deny {
		return d
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

// shortValued are, for commit and am, the short options that take the rest of their word or the
// next word as their value, or an attached one: commit's -m -F -C -c -t and -u -S, am's -C -p -S.
var shortValued = map[string]string{"commit": "mFCctuS", "am": "CpS"}

// skipsHooks reports whether a git command skips the hooks: --no-verify or a prefix of it, which
// git accepts (--no-veri); -n among the short options of commit or am, alone or in a cluster
// (-nm, -anm, but not -mn, whose n is the message); core.hooksPath, in any case, in git config.
func skipsHooks(sub string, rest []string) bool {
	// git config reads its key after a -- too.
	if sub == "config" && slices.ContainsFunc(rest, func(a string) bool { return strings.Contains(strings.ToLower(a), "core.hookspath") }) {
		return true
	}

	for _, a := range rest {
		if a == "--" {
			return false
		}

		name, _, _ := strings.Cut(a, "=")

		switch {
		case abbreviates(name, "--no-verify", len("--no-v")):
			return true
		case shortValued[sub] != "" && len(a) > 1 && a[0] == '-' && a[1] != '-':
			flags := a[1:]
			if cut := strings.IndexAny(flags, shortValued[sub]); cut >= 0 {
				flags = flags[:cut]
			}

			if strings.Contains(flags, "n") {
				return true
			}
		}
	}

	return false
}

// gitRuns judges what a git command runs on its caller's behalf: the shell lines of rebase --exec,
// submodule foreach, bisect run, difftool --extcmd and grep --open-files-in-pager, judged as lines,
// and for an agent the transport options, which run a program.
func (j *judge) gitRuns(sub string, rest []string) Decision {
	for _, l := range gitLines(sub, rest) {
		if d := j.child(l, maxDepth); d.Deny {
			return d
		}
	}

	if j.role.Agent() && transports(sub, rest) {
		return deny("this git option runs a program the policy cannot read: fetch and clone as they are configured")
	}

	return Allow
}

// gitLines are the shell lines a git command runs for its caller.
func gitLines(sub string, rest []string) []string {
	switch {
	case sub == "rebase":
		return optionValues(rest, 'x', "--exec")
	case sub == "difftool":
		return optionValues(rest, 'x', "--extcmd")
	case sub == subGrep:
		return optionValues(rest, 'O', "--open-files-in-pager")
	case sub == "submodule" && len(rest) > 0 && rest[0] == "foreach":
		return []string{strings.Join(skipOptions(rest[1:], nil), " ")}
	case sub == "bisect" && len(rest) > 0 && rest[0] == "run":
		return []string{strings.Join(rest[1:], " ")}
	}

	return nil
}

// transports reports whether a command that talks to a remote passes the program to run there or
// here: --upload-pack, --receive-pack, --exec (or a prefix git accepts), clone -u.
func transports(sub string, rest []string) bool {
	if !slices.Contains([]string{"clone", "fetch", "pull", "ls-remote", "archive", "submodule"}, sub) {
		return false
	}

	return slices.ContainsFunc(rest, func(a string) bool {
		name, _, _ := strings.Cut(a, "=")

		return (sub == "clone" && strings.HasPrefix(a, "-u")) ||
			slices.ContainsFunc([]string{"--upload-pack", "--receive-pack", "--exec"}, func(o string) bool { return abbreviates(name, o, len("--u")) })
	})
}

// abbreviates reports whether word is option, or a prefix of it no shorter than least, as git and
// make accept a long option cut short (--no-veri for --no-verify).
func abbreviates(word, option string, least int) bool {
	return len(word) >= least && strings.HasPrefix(option, word)
}

// optionValues are the values of a git option given as -xVALUE, -x VALUE (alone or ending a
// cluster), --long VALUE, --long=VALUE, or a prefix of --long that git accepts (--exe=VALUE).
func optionValues(args []string, short byte, long string) []string {
	var out []string

	for i := 0; i < len(args); i++ {
		a := args[i]
		name, value, attached := strings.Cut(a, "=")

		switch {
		case a == "--":
			return out
		case strings.HasPrefix(name, "--"):
			if !abbreviates(name, long, len("--ex")) {
				continue
			}
		case strings.HasPrefix(a, "-") && strings.IndexByte(a[1:], short) >= 0:
			value = a[strings.IndexByte(a, short)+1:]
			attached = value != ""
		default:
			continue
		}

		if !attached && i+1 < len(args) {
			i++
			value = args[i]
		}

		out = append(out, value)
	}

	return out
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

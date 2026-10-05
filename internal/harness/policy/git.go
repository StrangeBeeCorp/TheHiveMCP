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

// gitFlags are git's own options that take no value: any other option before the subcommand is
// refused, since one that takes the next word would make the policy read it as the subcommand.
var gitFlags = []string{
	"--version", "-v", optHelp, "-h", "-p", "--paginate", "-P", "--no-pager", "--no-replace-objects", "--no-lazy-fetch",
	"--no-optional-locks", "--no-advice", "--bare", "--literal-pathspecs", "--glob-pathspecs", "--noglob-pathspecs",
	"--icase-pathspecs", "--html-path", "--man-path", "--info-path", "--exec-path",
}

// git judges a git command: its own options first, then the subcommand.
func (j *judge) git(args []string) Decision {
	i := 0

	for i < len(args) && strings.HasPrefix(args[i], "-") {
		step, d := j.gitOption(args[i])
		if d.Deny {
			return d
		}

		i += step
	}

	if i >= len(args) {
		return Allow
	}

	return j.gitSub(args[i], args[i+1:])
}

// gitOption judges one of git's own options, and says how many words it takes: two for one whose
// value is the next word.
func (j *judge) gitOption(a string) (int, Decision) {
	switch {
	case a == "-c" || a == "--config-env" || strings.HasPrefix(a, "--config-env=") || strings.HasPrefix(a, "--exec-path="):
		return 0, deny("per-command git configuration is not allowed: the repository's configuration is policy")
	case slices.Contains([]string{"-C", "--git-dir", "--work-tree", "--namespace", "--super-prefix", "--attr-source", "--list-cmds"}, a):
		if j.role.Agent() && a != "-C" {
			return 0, deny("git runs on this checkout as it is configured")
		}

		return 2, Allow
	case !slices.Contains(gitFlags, a) && !strings.Contains(a, "="):
		return 0, deny("git %s is an option the policy does not know: write the command without it", a)
	case j.role.Agent() && (strings.HasPrefix(a, "--git-dir=") || strings.HasPrefix(a, "--work-tree=") || strings.HasPrefix(a, "--namespace=")):
		return 0, deny("git runs on this checkout as it is configured")
	}

	return 1, Allow
}

// gitCommand refuses a git command the policy cannot read as written: a word filled in as the line
// runs, or one that is not git's own (an alias, a git-<name> program).
func gitCommand(sub string) Decision {
	switch {
	case !plainWord(sub):
		return deny("git's command is %s, filled in as the line runs (find's {}, parallel's :::): write it out", sub)
	case !slices.Contains(gitCommands, sub):
		return deny("git %s is not one of git's own commands: an alias or a git-%s program runs words the policy does not read as theirs, so run the command it stands for", sub, sub)
	}

	return Allow
}

// pushes reports whether a git command pushes to a remote: push, send-pack, http-push, subtree's push.
func pushes(sub string, rest []string) bool {
	return slices.Contains([]string{subPush, "send-pack", "http-push"}, sub) || (sub == "subtree" && slices.Contains(rest, subPush))
}

func (j *judge) gitSub(sub string, rest []string) Decision {
	if d := gitCommand(sub); d.Deny {
		return d
	}

	if d := j.gitWritten(sub, rest); d.Deny {
		return d
	}

	if skipsHooks(sub, rest) {
		return deny("the git hooks are gates: commit without bypassing them, and fix what they report")
	}

	if d := j.gitRuns(sub, rest); d.Deny {
		return d
	}

	if pushes(sub, rest) {
		return deny("push with make pr: it runs the pr gates at HEAD, records the evidence and pushes this branch to " +
			"origin, never main and never the public mirror")
	}

	if movesMain(sub, rest) {
		return deny("main moves only when a person merges a pull request on GitHub")
	}

	if sub == subConfig {
		if d := j.gitConfig(rest); d.Deny {
			return d
		}
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
	if sub == subConfig && slices.ContainsFunc(rest, func(a string) bool { return strings.Contains(strings.ToLower(a), "core.hookspath") }) {
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

	// The main session may name the program a transport runs: it is judged as the line it is.
	if transports(sub, rest) {
		for _, l := range flagValues(rest, "", "u", "--upload-pack", "--receive-pack", "--exec") {
			if d := j.child(l, maxDepth); d.Deny {
				return d
			}
		}
	}

	return Allow
}

// gitWritten judges the files a git command writes besides the checkout.
func (j *judge) gitWritten(sub string, rest []string) Decision {
	for _, target := range gitWrites(sub, rest) {
		if d := j.write(target); d.Deny {
			return d
		}
	}

	return Allow
}

// gitWrites are the files a git command writes besides the checkout: --output of diff, show, log
// and their kin, format-patch's directory, archive's file, bundle create's.
func gitWrites(sub string, rest []string) []string {
	switch sub {
	case subDiff, subShow, "log", "whatchanged", "range-diff", "diff-tree", "diff-index", "diff-files":
		return flagValues(rest, "", "", "--output")
	case "format-patch":
		return flagValues(rest, "", "o", "--output-directory", "--output")
	case "archive":
		return flagValues(rest, "", "o", "--output")
	case "bundle":
		if len(rest) > 1 && rest[0] == "create" {
			return rest[1:2]
		}
	}

	return nil
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
	case sub == subModule && len(rest) > 0 && rest[0] == "foreach":
		return []string{strings.Join(skipOptions(rest[1:], nil), " ")}
	case sub == "bisect" && len(rest) > 0 && rest[0] == "run":
		return []string{strings.Join(rest[1:], " ")}
	case sub == "filter-branch":
		return flagValues(rest, "", "", "--setup", "--tree-filter", "--index-filter", "--parent-filter", "--msg-filter",
			"--commit-filter", "--tag-name-filter", "--env-filter")
	case sub == "send-email":
		return flagValues(rest, "", "", "--to-cmd", "--cc-cmd", "--header-cmd", "--sendmail-cmd", "--smtp-server")
	}

	return nil
}

// transports reports whether a command that talks to a remote passes the program to run there or
// here: --upload-pack, --receive-pack, --exec (or a prefix git accepts), clone -u.
func transports(sub string, rest []string) bool {
	if !slices.Contains([]string{"clone", subFetch, subPull, "ls-remote", "archive", subModule}, sub) {
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
	case subUpdateRef, "replace", "filter-branch", "filter-repo", "fast-import", "pack-refs", "symbolic-ref", "subtree":
		return deny("refs and history are the record of the work: they are not rewritten by hand")
	case "apply", "am", subTag, subFetch, subPull:
		return agentRefs(sub, rest)
	case "reflog":
		if len(rest) > 0 && rest[0] != subShow {
			return deny("refs and history are the record of the work: they are not rewritten by hand")
		}
	case "notes":
		if len(rest) > 0 && !slices.Contains([]string{subList, subShow}, rest[0]) {
			return deny("notes are written by the harness only")
		}
	case subConfig:
		if !slices.ContainsFunc(rest, func(a string) bool {
			return slices.Contains([]string{optGet, "--get-all", "--get-regexp", "--" + subList, "-l"}, a)
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

// agentRefs holds for an agent's patches, tags and fetches: a patch names the files it writes
// inside itself, tags are a person's, and a refspec of its own would move a ref the gates compare
// with.
func agentRefs(sub string, rest []string) Decision {
	switch sub {
	case "apply", "am":
		return deny("a patch names the files it writes inside itself: make the change with the editor, and leave patches to people")
	case subTag:
		if !agentListsTags(rest) {
			return deny("tags are made by people, when they release: list them with git tag -l")
		}
	default:
		if slices.ContainsFunc(rest, func(a string) bool { return strings.Contains(a, ":") && !strings.Contains(a, "://") }) {
			return deny("fetch as configured (git fetch origin): a refspec of your own moves a ref the gates compare with")
		}
	}

	return Allow
}

// readOnlyGit reports whether a subcommand outside gitReads only reads with these arguments.
func readOnlyGit(sub string, rest []string) bool {
	switch sub {
	case "branch":
		return len(rest) == 0 || slices.ContainsFunc(rest, func(a string) bool {
			return slices.Contains([]string{"--show-current", "--" + subList, "-l", "-a", "-r", "-v", "-vv", optContains, optMerged}, a)
		})
	case "remote":
		return len(rest) == 0 || rest[0] == "-v" || rest[0] == subShow || rest[0] == "get-url"
	case "worktree":
		return len(rest) > 0 && rest[0] == subList
	}

	return false
}

// movesMain reports whether a git command would rewrite the local main by hand. Main itself
// changes on GitHub only: no one pushes it from here (make pr refuses).
func movesMain(sub string, rest []string) bool {
	isMain := func(a string) bool { return a == mainBranch || strings.HasSuffix(a, "/"+mainBranch) }

	switch sub {
	case "branch":
		var positional []string

		for k := 0; k < len(rest); k++ {
			switch {
			case slices.Contains([]string{optContains, "--no-contains", optMerged, "--no-merged", "--points-at",
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
	case subUpdateRef, subTag, subFetch, subPull:
		return movesMainRef(sub, rest, isMain)
	}

	return false
}

// movesMainRef reports whether update-ref, tag or fetch would move main, or a ref that stands in for
// the remote's main (refs/remotes/origin/main, a tag spelt origin/main).
func movesMainRef(sub string, rest []string, isMain func(string) bool) bool {
	switch sub {
	case subUpdateRef:
		return slices.ContainsFunc(rest, func(a string) bool {
			return a == "refs/heads/"+mainBranch || a == mainBranch || (strings.HasPrefix(a, "refs/remotes/") && isMain(a))
		})
	case subTag:
		name := tagName(rest)
		return !tagLists(rest) && name != "" && isMain(name)
	}

	return slices.ContainsFunc(rest, func(a string) bool {
		_, dest, refspec := strings.Cut(strings.TrimPrefix(a, "+"), ":")
		return refspec && !strings.Contains(a, "://") && isMain(dest)
	})
}

// tagLists reports whether git tag lists tags: -l or --list, or one of the options git documents
// as implying it (-n, --contains, --no-contains, --merged, --no-merged, --points-at), or -v, which
// verifies. --sort, --format, --column and -i do not: with a name, git tag still makes the tag.
func tagLists(rest []string) bool {
	listing := []string{"-l", "--list", "-v", "--verify", optContains, "--no-contains", optMerged, "--no-merged", "--points-at"}

	return slices.ContainsFunc(rest, func(a string) bool {
		name, _, _ := strings.Cut(a, "=")
		return slices.Contains(listing, name) || (strings.HasPrefix(a, "-n") && !strings.HasPrefix(a, "--"))
	})
}

// agentListsTags reports whether an agent's git tag only lists or verifies: no word at all, or a
// listing option (tagLists) with the patterns or commits it reads, and no other option, since
// --sort, --format and their kin leave git tag making the tag it is named.
func agentListsTags(rest []string) bool {
	if slices.ContainsFunc(rest, func(a string) bool { return strings.HasPrefix(a, "-") && !tagLists([]string{a}) }) {
		return false
	}

	return len(rest) == 0 || tagLists(rest)
}

// tagName is the name git tag would give a tag: its first operand, past the values of -m, -F, -u
// and their long forms.
func tagName(rest []string) string {
	valued := []string{"-m", "-F", "-u", "--message", "--file", "--local-user", "--cleanup"}

	for i := 0; i < len(rest); i++ {
		switch a := rest[i]; {
		case a == "--":
			if i+1 < len(rest) {
				return rest[i+1]
			}

			return ""
		case slices.Contains(valued, a):
			i++
		case !strings.HasPrefix(a, "-"):
			return a
		}
	}

	return ""
}

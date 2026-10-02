package policy

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/shell"
)

// maxDepth bounds how many wrappers (sh -c, eval, env…) the policy unwraps.
const maxDepth = 8

var (
	shells       = []string{"sh", "bash", "zsh", "dash", "ksh", "fish"}
	interpreters = []string{
		"python", "python3", "perl", "ruby", "php", "lua", "node", "deno", "bun", "osascript", "expect",
		"tclsh", "source", ".", "uv", "uvx", "npx",
	}
	// wrappers run the command that follows their options; the listed options take a value.
	wrappers = map[string][]string{
		"nohup": nil, "nice": {"-n"}, "timeout": {"-s", "-k", "--signal", "--kill-after"},
		"stdbuf": {"-i", "-o", "-e"}, "command": nil, "builtin": nil, "exec": {"-a"}, "time": {"-f", "-o"},
		"watch": {"-n", "-d"}, "caffeinate": {"-t", "-w"}, "chronic": nil, "unbuffer": nil,
	}
	readCommands = []string{
		"ls", cmdCat, cmdHead, cmdTail, "wc", "grep", "rg", "find", "file", "stat", "pwd", cmdEcho, cmdPrintf, cmdSort,
		"uniq", subDiff, "tree", "jq", "yq", "true", "false", cmdSed, cmdAwk, "cut", "tr", "basename", "dirname",
		"realpath", "cd", "test", "[", "date", "which", "column", "less", "comm", "nl", "xxd", "od", "sha256sum",
		"shasum", "md5", cmdGit, "gh", cmdMake, "sleep",
	}
)

// Bash decides whether the role may run a command line in the checkout at root, from the
// directory cwd. Every command the line runs is judged, wrappers unwrapped; the first denial wins.
func Bash(role Role, root, cwd, line string) Decision {
	j := judge{role: role, root: filepath.Clean(root), cwd: filepath.Clean(cwd)}
	return j.line(line, 0)
}

type judge struct {
	role Role
	root string
	// cwd is where relative paths land; empty after a cd the policy could not follow.
	cwd string
	// tmpdirRebound is true once a line may set TMPDIR, which ${TMPDIR} then no longer names.
	tmpdirRebound bool
	// inChild is set while the command judged runs in a child shell of its line.
	inChild bool
	// passed is what the line may put in the environment of the commands it runs (passedBy).
	passed environment
	// written are where the line's earlier commands write, as they land: what a later copy in it
	// may carry was not on disk when the line was judged.
	written []string
}

func (j *judge) line(text string, depth int) Decision {
	if depth > maxDepth {
		return deny("commands nested this deep are not read by the policy: write the command plainly")
	}

	parsed := shell.Parse(text)
	if len(parsed.Opaque) > 0 && j.role.Agent() {
		return deny("the policy cannot read this command line (%s): write it plainly, or use a make target", parsed.Opaque[0])
	}

	j.tmpdirRebound = j.tmpdirRebound || rebinds(text, parsed, tmpdir)
	j.passed.add(passedBy(parsed, depth))

	for _, c := range parsed.Commands {
		d := j.command(c, depth)
		if d.Deny {
			return d
		}
	}

	return Allow
}

func (j *judge) command(c shell.Command, depth int) Decision {
	for _, a := range c.Assigns {
		d := j.assign(a)
		if d.Deny {
			return d
		}
	}

	for _, w := range c.Writes {
		d := j.write(w)
		if d.Deny {
			return d
		}
	}

	if d := j.reads(c); d.Deny {
		return d
	}

	if truncatesIntegration(c) {
		return deny("redirect the integration suite to a file (make test-integration > \"$TMPDIR/it.log\" 2>&1) " +
			"and grep it: cutting its output means running the slow suite again")
	}

	j.inChild = c.Child

	return j.run(c.Words, depth)
}

func truncatesIntegration(c shell.Command) bool {
	if len(c.Into) == 0 || len(c.Words) < 2 || path.Base(c.Words[0]) != cmdMake {
		return false
	}

	reader := path.Base(c.Into[0])

	return slices.Contains(c.Words[1:], "test-integration") && (reader == cmdHead || reader == cmdTail)
}

// assign judges NAME=value before a command or in export: git and make are configured by the
// repository, not per command.
func (j *judge) assign(a string) Decision {
	name, _, _ := strings.Cut(a, "=")
	if strings.HasPrefix(name, "GIT_CONFIG") || name == "GIT_EXEC_PATH" || name == "MAKEFLAGS" || name == "MAKEFILES" ||
		slices.Contains(configHomes, name) {
		return deny("environment that reconfigures git, gh or make for one command is not allowed: the repository's configuration is policy")
	}

	if j.role.Agent() && slices.Contains([]string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE"}, name) {
		return deny("git runs on this checkout as it is configured")
	}

	if j.role.Agent() && runsCode(a) {
		return deny("%s makes the next program run a command or load code the policy cannot read: run the commands themselves", name)
	}

	// The main session may set them: the command line it names is judged as the one it is.
	if _, value, _ := strings.Cut(a, "="); runsCode(a) && slices.Contains(commandRunners, name) {
		return j.child(value, maxDepth)
	}

	if _, value, _ := strings.Cut(a, "="); j.role.Agent() && name == "CDPATH" && value != "" {
		return deny("CDPATH sends cd where the policy cannot follow: cd by path")
	}

	return Allow
}

// run judges one command and its arguments, after unwrapping the wrappers.
func (j *judge) run(w []string, depth int) Decision {
	if len(w) == 0 {
		return Allow
	}

	name, args := w[0], w[1:]

	if strings.Contains(name, "/") {
		if isHarness(name) {
			return j.harness(args)
		}

		d := j.byPath(name)
		if d.Deny {
			return d
		}

		name = path.Base(name)
	}

	if d, done := j.unwrap(name, args, depth); done {
		return d
	}

	if d := j.rules(name, args); d.Deny {
		return d
	}

	return j.passedOn(name, args, depth)
}

// unwrap judges the commands that run other commands: shells, eval, env, xargs, find -exec and
// the wrappers. done is false for any other command.
func (j *judge) unwrap(name string, args []string, depth int) (Decision, bool) {
	switch {
	case name == "cd" || name == "pushd" || name == "popd":
		j.cd(name, args)
		return Allow, true
	case slices.Contains(shells, name):
		return j.shell(args, depth), true
	case name == "eval":
		return j.line(strings.Join(args, " "), depth+1), true
	case name == "env":
		return j.env(args, depth), true
	case name == "xargs":
		rest := skipOptions(args, []string{"-n", "-I", "-i", "-L", "-l", "-P", "-d", "-s", "-E", "-e", "-a", "--max-args", "--delimiter"})
		if len(rest) > 0 && harnessWord(rest[0]) {
			return deny("run bin/harness itself, not through xargs: xargs adds arguments the policy cannot read"), true
		}

		return j.run(rest, depth+1), true
	case name == "find":
		return j.find(args, depth), true
	case name == "harness":
		return j.harness(args), true
	}

	valued, ok := wrappers[name]
	if !ok {
		return Allow, false
	}

	rest := skipOptions(args, valued)
	if name == "timeout" && len(rest) > 0 {
		rest = rest[1:]
	}

	return j.run(rest, depth+1), true
}

// cd follows the directory cd or pushd moves to. One the policy cannot follow leaves it unknown,
// and relative paths are then refused to agents: popd, cd -, an option, a computed or ~ path, a
// CDPATH lookup, or a cd in a child shell, which leaves the rest of the line where it was.
func (j *judge) cd(name string, args []string) {
	dir := ""
	if len(args) > 0 {
		dir = args[0]
	}

	first := strings.SplitN(dir, "/", 2)[0]
	lookedUp := os.Getenv("CDPATH") != "" && !filepath.IsAbs(dir) && first != "." && first != ".."

	switch {
	case j.inChild || name == "popd" || dir == "" || j.cwd == "" || lookedUp || strings.HasPrefix(dir, "-") ||
		strings.HasPrefix(dir, "+") || strings.HasPrefix(dir, "~") || strings.Contains(dir, shell.Computed):
		j.cwd = ""
	case filepath.IsAbs(dir):
		j.cwd = filepath.Clean(dir)
	default:
		j.cwd = filepath.Join(j.cwd, dir)
	}
}

// child judges a line a child shell runs (sh -c, git rebase --exec): where it cds stays in it.
func (j *judge) child(line string, depth int) Decision {
	cwd, inChild := j.cwd, j.inChild
	defer func() { j.cwd, j.inChild = cwd, inChild }()

	return j.line(line, depth)
}

// shell judges a shell's -c script. Options end at the first word that is not one, which names a
// script: `bash gen.sh -c x` runs gen.sh, and -c x are its arguments.
func (j *judge) shell(args []string, depth int) Decision {
	for i := 0; i < len(args); i++ {
		a := args[i]

		switch {
		case a == "--" || (!strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "+")):
			i = len(args)
		case a == "-o" || a == "+o" || a == "-O" || a == "+O":
			i++
		case strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "c"):
			if i+1 < len(args) {
				return j.child(args[i+1], depth+1)
			}

			return Allow
		}
	}

	if j.role.Agent() {
		return deny("a shell running a script or its input is opaque to the policy: run the commands, or the make target that runs them")
	}

	return Allow
}

func (j *judge) env(args []string, depth int) Decision {
	if slices.ContainsFunc(args, func(a string) bool { return strings.HasPrefix(a, "-S") || a == "--split-string" }) {
		return deny("env -S splits a string into a command the policy cannot read")
	}

	rest := skipOptions(args, []string{"-u", "--unset", "-C", "--chdir"})
	for len(rest) > 0 && strings.Contains(rest[0], "=") && !strings.HasPrefix(rest[0], "-") {
		d := j.assign(rest[0])
		if d.Deny {
			return d
		}

		j.passed.assigns = append(j.passed.assigns, rest[0])
		rest = rest[1:]
	}

	return j.run(rest, depth+1)
}

// find judges the commands find -exec runs, and the files its actions delete or write.
func (j *judge) find(args []string, depth int) Decision {
	for i := 0; i < len(args); i++ {
		var d Decision

		switch args[i] {
		case "-exec", "-execdir", "-ok", "-okdir":
			end := i + 1
			for end < len(args) && args[end] != ";" && args[end] != "+" {
				end++
			}

			d = j.run(args[i+1:end], depth+1)
			i = end
		case "-delete", "-fprint", "-fprint0", "-fprintf", "-fls":
			d = j.findWrite(args, i)
		}

		if d.Deny {
			return d
		}
	}

	return Allow
}

// findWrite judges the action of find at i that changes files: -delete, or the file -fprint,
// -fprintf or -fls writes.
func (j *judge) findWrite(args []string, i int) Decision {
	if j.role.ReadOnly() || j.role.Duel() {
		return deny("find %s changes files: the %s may not", args[i], j.role)
	}

	if args[i] == "-delete" || i+1 >= len(args) {
		return Allow
	}

	return j.write(args[i+1])
}

// byPath judges a command named by its path: a script in the checkout or in a temporary
// directory is code the policy cannot read, except bin/harness.
func (j *judge) byPath(name string) Decision {
	if !j.role.Agent() {
		return Allow
	}

	if j.cwd == "" && !filepath.IsAbs(name) {
		return deny("a relative command after a cd the policy could not follow: run it from the checkout")
	}

	abs := name
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(j.cwd, name)
	}

	if _, inside := j.landed(abs); inside || temporary(landing(abs)) {
		return deny("%s is a script the policy cannot read: run the make target that wraps it (make help)", name)
	}

	return Allow
}

func isHarness(name string) bool {
	return name == "bin/harness" || name == "./bin/harness" || strings.HasSuffix(name, "/bin/harness")
}

// harnessWord reports whether word runs the harness: bin/harness by any path, or harness itself.
func harnessWord(word string) bool {
	return isHarness(word) || word == "harness"
}

// write judges a file the command line writes, relative to where the line has cd'ed. A path the
// shell computes as the line runs is judged only as ${TMPDIR}/…: an agent writes nowhere else the
// policy cannot read. A pattern is judged by the files it matches now, and by its own text, which
// the shell passes on when nothing matches.
func (j *judge) write(target string) Decision {
	if strings.HasPrefix(target, "/dev/") {
		return Allow
	}

	target, _ = j.resolveTemp(target, false)
	if strings.Contains(target, shell.Computed) {
		if j.role.Agent() {
			return deny("%s is computed as the line runs, so the policy cannot judge it: name the file, or write under \"$TMPDIR\"", target)
		}

		return Allow
	}

	if !filepath.IsAbs(target) && !strings.HasPrefix(target, "~") {
		if j.cwd == "" {
			if j.role.Agent() {
				return deny("a relative write after a cd the policy could not follow: use a path from the checkout")
			}

			return Allow
		}

		target = filepath.Join(j.cwd, target)
	}

	j.written = append(j.written, landing(target))

	if !strings.ContainsAny(target, "*?[") {
		return Write(j.role, j.root, target)
	}

	matches, _ := filepath.Glob(strings.ReplaceAll(target, "[!", "[^"))
	for _, m := range append(matches, target) {
		if d := Write(j.role, j.root, m); d.Deny {
			return d
		}
	}

	return Allow
}

// skipOptions returns the arguments after a command's options; valued options take the next word.
func skipOptions(args, valued []string) []string {
	i := 0
	for i < len(args) {
		a := args[i]
		if a == "--" {
			return args[i+1:]
		}

		if !strings.HasPrefix(a, "-") || a == "-" {
			break
		}

		i++
		if slices.Contains(valued, a) {
			i++
		}
	}

	if i > len(args) {
		return nil
	}

	return args[i:]
}

package policy

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
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
		"ls", "cat", "head", "tail", "wc", "grep", "rg", "find", "file", "stat", "pwd", "echo", cmdPrintf, "sort",
		"uniq", subDiff, "tree", "jq", "yq", "true", "false", cmdSed, "awk", "cut", "tr", "basename", "dirname",
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

	return slices.Contains(c.Words[1:], "test-integration") && (reader == "head" || reader == "tail")
}

// assign judges NAME=value before a command or in export: git and make are configured by the
// repository, not per command.
func (j *judge) assign(a string) Decision {
	name, _, _ := strings.Cut(a, "=")
	if strings.HasPrefix(name, "GIT_CONFIG") || name == "GIT_EXEC_PATH" || name == "MAKEFLAGS" || name == "MAKEFILES" {
		return deny("environment that reconfigures git or make for one command is not allowed: the repository's configuration is policy")
	}

	if name == claudeCode {
		return deny("CLAUDECODE tells the harness a person is not at the keyboard: it stays as Claude Code sets it")
	}

	if j.role.Agent() && slices.Contains([]string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE"}, name) {
		return deny("git runs on this checkout as it is configured")
	}

	if j.role.Agent() && runsCode(a) {
		return deny("%s makes the next program run a command or load code the policy cannot read: run the commands themselves", name)
	}

	if _, value, _ := strings.Cut(a, "="); j.role.Agent() && name == "CDPATH" && value != "" {
		return deny("CDPATH sends cd where the policy cannot follow: cd by path")
	}

	return Allow
}

// codeRunners are the variables that make a program run another command or load code: git's
// pager, editor, diff and transport, the dynamic loader's, a shell's start-up file, PATH.
var codeRunners = []string{
	"GIT_EXTERNAL_DIFF", "GIT_PAGER", "PAGER", "GIT_EDITOR", "GIT_SEQUENCE_EDITOR", "EDITOR", "VISUAL", "GIT_SSH",
	"GIT_SSH_COMMAND", "GIT_ASKPASS", "SSH_ASKPASS", "GIT_PROXY_COMMAND", "LESSOPEN", "LESSCLOSE", "BASH_ENV", "ENV",
	"PROMPT_COMMAND", "LD_PRELOAD", "LD_LIBRARY_PATH", "LD_AUDIT", "PATH", "SHELL",
}

// runsCode reports whether NAME=value sets a code runner to anything but a no-op (GIT_PAGER=cat,
// GIT_EDITOR=true).
func runsCode(a string) bool {
	name, value, _ := strings.Cut(a, "=")
	if !slices.Contains(codeRunners, name) && !strings.HasPrefix(name, "DYLD_") {
		return false
	}

	return !slices.Contains([]string{"", "cat", cmdTrue, ":"}, value)
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

// passedOn judges what a command the policy has no rule for may run on its caller's behalf:
// script, setsid, flock, parallel, arch and many more take a command as their arguments. From each
// argument that names a guarded command, the rest of the line is judged as that command, and an
// argument that is a command line starting with one is judged as a line.
func (j *judge) passedOn(name string, args []string, depth int) Decision {
	if slices.Contains(readCommands, name) {
		return Allow
	}

	for i, a := range args {
		first, _, spaced := strings.Cut(strings.TrimSpace(a), " ")

		var d Decision

		switch {
		case guarded(a):
			d = j.run(args[i:], depth+1)
		case spaced && guarded(first):
			d = j.line(a, depth+1)
		}

		if d.Deny {
			return d
		}
	}

	return Allow
}

// guarded reports whether word names a command the policy has rules for, or one that runs others:
// git, gh, make, the harness, env, eval, xargs, a shell, an interpreter or a wrapper. A path names
// one only from a bin directory (/usr/bin/git), not a file of the checkout (.cache/tmp/git).
func guarded(word string) bool {
	if isHarness(word) {
		return true
	}

	name := path.Base(word)
	if strings.Contains(word, "/") && !slices.Contains([]string{"bin", "sbin"}, path.Base(path.Dir(word))) {
		return false
	}

	if _, ok := wrappers[name]; ok {
		return true
	}

	return slices.Contains([]string{cmdGit, "gh", cmdMake, "env", "eval", "xargs"}, name) || slices.Contains(shells, name) ||
		(slices.Contains(interpreters, name) && name != "." && name != "source")
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

	if name == "exec" && slices.Contains(args, "-c") {
		return deny("exec -c empties the environment Claude Code sets: run the command as it is"), true
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

	if clearsClaudeCode(args) {
		return deny("CLAUDECODE tells the harness a person is not at the keyboard: the environment stays as Claude Code sets it")
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

func (j *judge) find(args []string, depth int) Decision {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-exec", "-execdir", "-ok", "-okdir":
			end := i + 1
			for end < len(args) && args[end] != ";" && args[end] != "+" {
				end++
			}

			d := j.run(args[i+1:end], depth+1)
			if d.Deny {
				return d
			}

			i = end
		case "-delete", "-fprint", "-fprintf", "-fls":
			if j.role.ReadOnly() || j.role.Duel() {
				return deny("find %s changes files: the %s may not", args[i], j.role)
			}
		}
	}

	return Allow
}

// tmpdir is the variable that names the temporary directory, where agents write their logs.
const tmpdir = "TMPDIR"

// varWriters are the builtins that set a variable they are given by name.
var varWriters = []string{"read", cmdPrintf, "mapfile", "readarray", "getopts", "export", "unset"}

// declarers are the builtins that declare variables, namerefs (-n) among them.
var declarers = regexp.MustCompile(`(^|[^A-Za-z0-9_])(declare|local|typeset|readonly)([^A-Za-z0-9_]|$)`)

// rebinds reports whether a line may set the variable, judged wide: any mention of its name but
// its own expansions ($NAME, ${NAME}) in the text or in the words once quotes are joined (an
// assignment, a for or select loop, read, export, printf -v, unset, a nameref's target), any
// declaration (a nameref may point at it), and a builtin that sets a variable named at run time.
func rebinds(text string, line shell.Line, name string) bool {
	if mentions(text, name) || declarers.MatchString(text) {
		return true
	}

	for _, c := range line.Commands {
		if slices.ContainsFunc(c.Assigns, func(a string) bool { return mentions(a, name) }) ||
			slices.ContainsFunc(c.Words, func(w string) bool { return mentions(w, name) }) {
			return true
		}

		writer := len(c.Words) > 0 && slices.Contains(varWriters, path.Base(c.Words[0]))
		if writer && slices.ContainsFunc(c.Words[1:], func(w string) bool { return strings.Contains(w, shell.Computed) }) {
			return true
		}
	}

	return false
}

// mentions reports whether s names the variable other than by expanding it.
func mentions(s, name string) bool {
	return strings.Count(s, name) > strings.Count(s, "$"+name)+strings.Count(s, "${"+name+"}")
}

// clearsClaudeCode reports whether env's options drop CLAUDECODE: -u CLAUDECODE, or the whole
// environment.
func clearsClaudeCode(args []string) bool {
	for i, a := range args {
		switch {
		case a == "-i" || a == "--ignore-environment" || a == "-" || a == "--unset="+claudeCode || a == "-u"+claudeCode:
			return true
		case (a == "-u" || a == "--unset") && i+1 < len(args) && args[i+1] == claudeCode:
			return true
		}
	}

	return false
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

	if _, inside := relative(j.root, abs); inside || temporary(abs) {
		return deny("%s is a script the policy cannot read: run the make target that wraps it (make help)", name)
	}

	return Allow
}

func isHarness(name string) bool {
	return name == "bin/harness" || name == "./bin/harness" || strings.HasSuffix(name, "/bin/harness")
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

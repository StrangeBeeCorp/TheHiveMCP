package policy

import (
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
		"ls", "cat", "head", "tail", "wc", "grep", "rg", "find", "file", "stat", "pwd", "echo", "printf", "sort",
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
}

func (j *judge) line(text string, depth int) Decision {
	if depth > maxDepth {
		return deny("commands nested this deep are not read by the policy: write the command plainly")
	}

	parsed := shell.Parse(text)
	if len(parsed.Opaque) > 0 && j.role.Agent() {
		return deny("the policy cannot read this command line (%s): write it plainly, or use a make target", parsed.Opaque[0])
	}

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

	if truncatesIntegration(c) {
		return deny("redirect the integration suite to a file (make test-integration > \"$TMPDIR/it.log\" 2>&1) " +
			"and grep it: cutting its output means running the slow suite again")
	}

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

	if j.role.Agent() && slices.Contains([]string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE"}, name) {
		return deny("git runs on this checkout as it is configured")
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

	return j.rules(name, args)
}

// unwrap judges the commands that run other commands: shells, eval, env, xargs, find -exec and
// the wrappers. done is false for any other command.
func (j *judge) unwrap(name string, args []string, depth int) (Decision, bool) {
	switch {
	case name == "cd":
		j.cd(args)
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

	rest := skipOptions(args, valued)
	if name == "timeout" && len(rest) > 0 {
		rest = rest[1:]
	}

	return j.run(rest, depth+1), true
}

func (j *judge) cd(args []string) {
	switch {
	case len(args) == 0 || j.cwd == "" || strings.Contains(args[0], shell.Computed) || strings.HasPrefix(args[0], "~"):
		j.cwd = ""
	case filepath.IsAbs(args[0]):
		j.cwd = filepath.Clean(args[0])
	default:
		j.cwd = filepath.Join(j.cwd, args[0])
	}
}

func (j *judge) shell(args []string, depth int) Decision {
	for i, a := range args {
		if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "c") {
			if i+1 < len(args) {
				return j.line(args[i+1], depth+1)
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

// write judges a file the command line writes, relative to where the line has cd'ed.
func (j *judge) write(target string) Decision {
	if strings.HasPrefix(target, "/dev/") {
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

	return Write(j.role, j.root, target)
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

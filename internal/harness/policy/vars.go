package policy

import (
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/shell"
)

// commandRunners are the variables whose value a program runs as a command line: git's and gh's
// pager, editor, diff, transport and browser, less's filters, the shell's prompt command.
var commandRunners = []string{
	"GIT_EXTERNAL_DIFF", "GIT_PAGER", "PAGER", "GIT_EDITOR", "GIT_SEQUENCE_EDITOR", "EDITOR", "VISUAL", "GIT_SSH",
	"GIT_SSH_COMMAND", "GIT_ASKPASS", "SSH_ASKPASS", "GIT_PROXY_COMMAND", "LESSOPEN", "LESSCLOSE", "PROMPT_COMMAND",
	"GH_PAGER", "GH_EDITOR", "GH_BROWSER", "BROWSER",
}

// codeRunners are the variables that make a program run another command or load code: the
// command runners, the dynamic loader's, a shell's start-up file, PATH.
var codeRunners = append(slices.Clone(commandRunners), "BASH_ENV", "ENV", "LD_PRELOAD", "LD_LIBRARY_PATH", "LD_AUDIT", "PATH", "SHELL")

// configHomes are the variables that point git and gh at another configuration, whose aliases and
// programs the policy never read.
var configHomes = []string{"HOME", "XDG_CONFIG_HOME", "GH_CONFIG_DIR"}

// runsCode reports whether NAME=value sets a code runner to anything but a no-op (GIT_PAGER=cat,
// GIT_EDITOR=true).
func runsCode(a string) bool {
	name, value, _ := strings.Cut(a, "=")
	if !slices.Contains(codeRunners, name) && !strings.HasPrefix(name, "DYLD_") {
		return false
	}

	return !slices.Contains([]string{"", cmdCat, cmdTrue, ":"}, value)
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

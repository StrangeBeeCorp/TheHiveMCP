package policy

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/shell"
)

// rawTools run pinned in Docker through the Makefile; the hint says which target.
var rawTools = map[string]string{
	"go":            "make test, make build, make run ARGS=…, make updatedep: Go runs in the Makefile's pinned image",
	"gofmt":         hintFmt,
	"gofumpt":       hintFmt,
	"golangci-lint": "make lint, make lint-changed or make lint-fix",
	"gosec":         "make sast",
	"govulncheck":   "make vulncheck",
	"gitleaks":      "make secrets",
	"hadolint":      "make dockerlint",
	"trivy":         "make dockersec",
	"shellcheck":    hintLint,
	"shfmt":         hintFmt,
	"markdownlint":  hintLint,
	"prettier":      hintFmt,
	"actionlint":    hintLint,
	"checkmake":     "make lint-makefile",
	"yamllint":      hintLint,
	"lychee":        hintLint,
}

// agentDenied are the commands no agent runs, with why.
var agentDenied = map[string]string{
	"patch":    "a patch names the files it writes inside itself: make the change with the editor",
	"security": "the keychain is the user's: agents never read it",
	"ssh":      noNetwork,
	"scp":      noNetwork,
	"sftp":     noNetwork,
	"nc":       noNetwork,
	"ncat":     noNetwork,
	"telnet":   noNetwork,
	"ftp":      noNetwork,
}

var dockerReads = []string{"ps", "logs", "inspect", "images", subVersion, "info"}

// rules judge a command once wrappers are unwrapped.
func (j *judge) rules(name string, args []string) Decision {
	if d := j.everyone(name, args); d.Deny {
		return d
	}

	switch name {
	case cmdMake:
		return j.make(args)
	case cmdGit:
		return j.git(args)
	case "gh":
		return j.gh(args)
	}

	if j.role.Agent() {
		if d := agentOnly(name, args); d.Deny {
			return d
		}
	}

	if j.role.ReadOnly() && (!slices.Contains(readCommands, name) || inPlace(name, args)) {
		return deny("the %s only reads: %s is not a read command", j.role, name)
	}

	for _, target := range targets(name, args) {
		if d := j.write(target); d.Deny {
			return d
		}
	}

	return j.trees(name, args)
}

// noFiles are commands that read no file their arguments name.
var noFiles = []string{"echo", cmdPrintf, "true", "false", "test", "[", "cd", "pwd", "date", "which", "sleep", "basename", "dirname", "export"}

// reads refuses a command that may read what the role may not (Read): the canaries for an agent,
// the sealed tests too for the Coder. A word that names a path, from the directory the line is
// in, is judged as it is; one the shell will expand (an unquoted glob, a variable, a substitution)
// is refused when what it could expand to reaches a hidden directory, and so is a command run from
// inside one. It is a deny-list over the parsed line: a recursive read or copy of a directory
// above them (grep -r x ., tar -cf - .claude), or a program that finds files by its own logic, is
// not caught. It keeps an agent that follows its instructions away from them, not one set on
// reading them.
func (j *judge) reads(c shell.Command) Decision {
	hidden := hiddenFrom(j.role)
	if len(hidden) == 0 {
		return Allow
	}

	if rel, inside := relative(j.root, j.cwd); j.cwd != "" && inside && underAny(rel, hidden) {
		return deny("%s is hidden from the %s: run the command from the checkout", rel, j.role)
	}

	for _, w := range readWords(c) {
		text, ok := optionValue(w.text)
		if !ok {
			continue
		}

		if d := j.readWord(text, w.exact, hidden); d.Deny {
			return d
		}
	}

	return Allow
}

// optionValue is the path a word may name: itself, the value of --opt=value or -ovalue, or none
// for a bare option.
func optionValue(w string) (string, bool) {
	switch {
	case !strings.HasPrefix(w, "-"):
		return w, true
	case strings.Contains(w, "="):
		_, value, _ := strings.Cut(w, "=")
		return value, true
	case !strings.HasPrefix(w, "--") && len(w) > len("-o"):
		return w[len("-o"):], true
	}

	return "", false
}

// word is a word of a command line, and whether it is known before the line runs.
type word struct {
	text  string
	exact bool
}

// readWords are the words a command may read: its input redirections, and its arguments unless
// it reads no file.
func readWords(c shell.Command) []word {
	words := make([]word, 0, len(c.Reads)+len(c.Words))
	for _, r := range c.Reads {
		words = append(words, word{text: r, exact: !strings.ContainsAny(r, "*?["+shell.Computed)})
	}

	if len(c.Words) == 0 || slices.Contains(noFiles, path.Base(c.Words[0])) {
		return words
	}

	for i, w := range c.Words[1:] {
		words = append(words, word{text: w, exact: i+1 < len(c.Exact) && c.Exact[i+1]})
	}

	return words
}

// readWord judges one word a command may read: a path as it is, a pattern by where it may lead.
func (j *judge) readWord(w string, exact bool, hidden []string) Decision {
	w, exact = j.resolveTemp(w, exact)

	if j.cwd == "" && !filepath.IsAbs(w) && strings.ContainsAny(w, "/.*?["+shell.Computed) {
		return deny("after a cd the policy could not follow, the %s names a file by its full path: %s", j.role, w)
	}

	if cut := strings.IndexAny(w, "*?["+shell.Computed); !exact && cut >= 0 {
		return j.readPattern(w, cut, hidden)
	}

	return Read(j.role, j.root, j.abs(w))
}

// resolveTemp reads ${TMPDIR} as the temporary directory, unless the line may rebind it.
func (j *judge) resolveTemp(w string, exact bool) (string, bool) {
	rest, ok := strings.CutPrefix(w, "${"+tmpdir+"}")
	if !ok || j.tmpdirRebound {
		return w, exact
	}

	w = filepath.Clean(os.TempDir()) + rest

	return w, !strings.ContainsAny(w, "*?["+shell.Computed)
}

// readPattern judges a word the shell will expand, from its text up to the first wildcard or
// variable: refused when it may expand into a hidden directory, or anywhere through a later ..
func (j *judge) readPattern(w string, cut int, hidden []string) Decision {
	if strings.Contains(w[cut:], "..") {
		return deny("%s may lead anywhere through its ..: name the file", w)
	}

	rel, inside := relative(j.root, j.abs(w[:cut]))
	if !inside {
		return Allow
	}

	if rel == "." {
		rel = ""
	}

	for _, h := range hidden {
		if strings.HasPrefix(h, rel) || under(rel, h) {
			return deny("%s may expand into %s, which is hidden from the %s: name the files you need", w, h, j.role)
		}
	}

	return Allow
}

// abs is where a word of the command line points, from the directory the line has cd'ed to.
func (j *judge) abs(w string) string {
	if filepath.IsAbs(w) || j.cwd == "" {
		return w
	}

	return filepath.Join(j.cwd, w)
}

// everyone holds for the main session too.
func (j *judge) everyone(name string, args []string) Decision {
	if name == "sudo" || name == "doas" {
		return deny("no sudo here: everything runs as the user, in containers")
	}

	if name == "unset" && slices.Contains(args, claudeCode) {
		return deny("CLAUDECODE tells the harness a person is not at the keyboard: it stays as Claude Code sets it")
	}

	if hint, ok := rawTools[name]; ok {
		return deny("%s runs pinned in Docker: use %s", name, hint)
	}

	if strings.HasPrefix(name, "markdownlint") {
		return deny("markdownlint runs pinned in Docker: use " + hintLint)
	}

	for _, a := range args {
		if slices.ContainsFunc(strings.FieldsFunc(a, func(r rune) bool { return r == ':' || r == '=' }), IsEnvFile) {
			return deny("environment files hold secrets and nobody reads them: the keys are listed in .env.template")
		}
	}

	return Allow
}

// agentOnly holds for every agent, not for the main session.
func agentOnly(name string, args []string) Decision {
	if why, ok := agentDenied[name]; ok {
		return deny("%s", why)
	}

	switch {
	case slices.Contains(interpreters, name):
		return deny("%s runs code the policy cannot read: use the editor for files and the make targets for the rest", name)
	case name == "awk" && slices.ContainsFunc(args, awkRuns):
		return deny("this awk program runs commands or writes files, which the policy cannot read")
	case name == "docker" && (len(args) == 0 || !slices.Contains(dockerReads, args[0])):
		return deny("containers are run by the make targets (make help): agents only read docker's state")
	case (name == "curl" || name == "wget") && !slices.ContainsFunc(args, local):
		return deny("no network from the shell: read documentation with WebFetch")
	}

	return Allow
}

func awkRuns(program string) bool {
	return strings.Contains(program, "system") || strings.Contains(program, "getline") ||
		strings.Contains(program, "|") || strings.Contains(program, "print >")
}

func local(url string) bool {
	return strings.Contains(url, "localhost") || strings.Contains(url, "127.0.0.1")
}

// inPlace reports whether a read command is used to write: sed -i.
func inPlace(name string, args []string) bool {
	return name == cmdSed && slices.ContainsFunc(args, func(a string) bool {
		return strings.HasPrefix(a, "-i") || strings.HasPrefix(a, "--in-place")
	})
}

// targets are the files a writing command names.
func targets(name string, args []string) []string {
	var positional []string

	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			positional = append(positional, a)
		}
	}

	switch name {
	case "tee", "rm", "rmdir", "touch", "chmod", "chown", "truncate", "mkdir", "unlink", "shred", "ln":
		return positional
	case "cp", "mv", "install", "rsync":
		if dir := valueOf(args, "-t", "--target-directory"); dir != nil {
			return dir
		}

		if len(positional) > 0 {
			return positional[len(positional)-1:]
		}
	case cmdSed:
		if inPlace(name, args) && len(positional) > 1 {
			return positional[1:]
		}
	case "dd":
		for _, a := range args {
			if out, ok := strings.CutPrefix(a, "of="); ok {
				return []string{out}
			}
		}
	case "tar", "unzip":
		return valueOf(args, "-C", "-d", "--directory")
	}

	return nil
}

// valueOf is the word after the first of the flags, if any.
func valueOf(args []string, flags ...string) []string {
	for i, a := range args {
		if slices.Contains(flags, a) && i+1 < len(args) {
			return []string{args[i+1]}
		}
	}

	return nil
}

// harness judges bin/harness: pushing is for the sessions that ship, gates for those that edit.
func (j *judge) harness(args []string) Decision {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}

	switch {
	case (sub == subPush || sub == "pr") && (j.role.Duel() || j.role.ReadOnly()):
		return deny("the %s does not push: the session that runs the duel ships the change", j.role)
	case j.role.ReadOnly() && !slices.Contains([]string{"select", subList, "check", "hook"}, sub):
		return deny("the %s reads and reports; it does not run gates", j.role)
	}

	return Allow
}

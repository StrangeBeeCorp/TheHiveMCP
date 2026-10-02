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

// variants are the other names of the commands the rules read: GNU's sed and awk, and other awks.
var variants = map[string]string{"gsed": cmdSed, "gawk": cmdAwk, "mawk": cmdAwk, "nawk": cmdAwk}

// rules judge a command once wrappers are unwrapped.
func (j *judge) rules(name string, args []string) Decision {
	if canonical, ok := variants[name]; ok {
		name = canonical
	}

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

	if d := j.writesOf(name, args); d.Deny {
		return d
	}

	return j.trees(name, args)
}

// writesOf judges what a command writes and runs besides its output: the files it names, a tree
// it brings over the records, the programs a sed or awk script runs, a link to a guarded place.
func (j *judge) writesOf(name string, args []string) Decision {
	for _, target := range targets(name, args) {
		if d := j.write(target); d.Deny {
			return d
		}

		if d := j.copyOverRecords(name, args, target); d.Deny {
			return d
		}
	}

	if d := j.embedded(name, args); d.Deny {
		return d
	}

	return j.linksGuarded(name, args)
}

// personDenied is why no Claude role passes --person, the harness flag only a person types.
const personDenied = "--person is a person's own word at their terminal: from a Claude session the push needs make pr's evidence and a ready review of HEAD"

// printers print or search the words they are given and run nothing: a --person among them is
// text (grep -rn -- --person cmd), not the harness's flag.
var printers = []string{cmdEcho, cmdPrintf, "grep", "rg", cmdCat, cmdHead, cmdTail, "wc", "less", "ls", subDiff, "comm", "nl", "column", "cut", "tr"}

// personFlag reports whether a word passes the harness's --person, alone or inside a value
// (make go CMD="run ./cmd/harness push --person").
func personFlag(word string) bool {
	fields := strings.FieldsFunc(word, func(r rune) bool { return strings.ContainsRune(" \t\n=\"'`(){};,|&", r) })

	return slices.ContainsFunc(fields, func(f string) bool { return f == "--person" || f == "-person" })
}

// noFiles are commands that read no file their arguments name.
var noFiles = []string{cmdEcho, cmdPrintf, "true", "false", "test", "[", "cd", "pwd", "date", "which", "sleep", "basename", "dirname", "export"}

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

	if rel, inside := j.landed(j.cwd); j.cwd != "" && inside && underAny(rel, hidden) {
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

	if foldsToASCII(w) {
		return deny(lookAlike, w)
	}

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

	rel, inside := j.landed(j.abs(w[:cut]))
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

// landed is where abs lands in the checkout, as the refusals compare it: through its symlinks, in
// lower case (the hidden directories and the records are named in lower case), and whether it is
// inside the checkout at all.
func (j *judge) landed(abs string) (string, bool) {
	if !filepath.IsAbs(abs) {
		return "", false
	}

	rel, inside := relative(landing(j.root), landing(abs))

	return strings.ToLower(rel), inside
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

	if !slices.Contains(append([]string{cmdGit, "gh"}, printers...), name) && slices.ContainsFunc(args, personFlag) {
		return deny(personDenied)
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
	case name == cmdAwk && slices.ContainsFunc(args, awkRuns):
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

// harness judges bin/harness: make pr is how the sessions that ship push, and gates are for those
// that edit. bin/harness push, which trusts a recorded verdict, is a person's.
func (j *judge) harness(args []string) Decision {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}

	switch {
	case slices.ContainsFunc(args, personFlag):
		return deny(personDenied)
	case slices.ContainsFunc(args, func(a string) bool {
		return strings.Contains(a, shell.Computed) || strings.ContainsAny(a, "*?[") || a == "{}"
	}):
		return deny("write the harness's arguments out: the policy reads them before it runs, and cannot read a variable, a substitution, a glob or find's {}")
	case sub == subPush:
		return deny("push with make pr: it runs the pr gates at HEAD and pushes in the same run, so no earlier record stands in for them")
	case sub == "pr" && (j.role.Duel() || j.role.ReadOnly()):
		return deny("the %s does not push: the session that runs the duel ships the change", j.role)
	case j.role.ReadOnly() && !slices.Contains([]string{"select", subList, "check", "hook"}, sub):
		return deny("the %s reads and reports; it does not run gates", j.role)
	}

	return Allow
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
		if d := j.handedOn(name, args, i, a, depth); d.Deny {
			return d
		}

		// An option may hand it a command after an = (tar --to-command=CMD, --checkpoint-action=exec=CMD).
		for k := strings.IndexByte(a, '='); k >= 0; k = nextByte(a, '=', k) {
			if d := j.handedOn(name, nil, 0, a[k+1:], depth); d.Deny {
				return d
			}
		}
	}

	return Allow
}

// handedOn judges one word a command the policy has no rule for is handed: as the command it names
// with the rest of args after it, or as a line when it is one.
func (j *judge) handedOn(name string, args []string, i int, a string, depth int) Decision {
	first, _, spaced := strings.Cut(strings.TrimSpace(a), " ")

	switch {
	case harnessWord(a) || (spaced && harnessWord(first)):
		return deny("run bin/harness itself, not through %s: a command that runs it may add arguments the policy cannot read", name)
	case guarded(a) && args != nil:
		return j.run(args[i:], depth+1)
	case spaced && guarded(first), guarded(a):
		return j.line(a, depth+1)
	}

	return Allow
}

// nextByte is the index of the next c in s after k, or -1.
func nextByte(s string, c byte, k int) int {
	n := strings.IndexByte(s[k+1:], c)
	if n < 0 {
		return -1
	}

	return k + 1 + n
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

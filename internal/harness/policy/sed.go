package policy

import (
	"regexp"
	"slices"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/shell"
)

// sedCall is what a sed command does besides printing: the files it edits in place, those its
// script writes (w, W, the w flag of s), the command lines it runs (e command), and whether it runs
// what it reads (e alone, the e flag of s) or a script the policy has not read (-f).
type sedCall struct {
	inPlace          bool
	files, writes    []string
	runs, scripts    []string
	exec, scriptFile bool
}

// parseSed reads sed's arguments as GNU's sed and as BSD's (this host's) would, and keeps what
// either would do. They differ on -i, whose next word BSD's always takes as the backup suffix, and
// on bracket expressions, inside which BSD's reads the delimiter as a character ([/]).
func parseSed(args []string) sedCall {
	gnu, bsd := parseSedAs(args, false), parseSedAs(args, true)

	return sedCall{
		inPlace: gnu.inPlace || bsd.inPlace, files: append(gnu.files, bsd.files...), writes: append(gnu.writes, bsd.writes...),
		runs: append(gnu.runs, bsd.runs...), scripts: append(gnu.scripts, bsd.scripts...), exec: gnu.exec || bsd.exec,
		scriptFile: gnu.scriptFile || bsd.scriptFile,
	}
}

// parseSedAs reads sed's arguments as one sed does: -e and -f in a cluster or alone, their long
// forms and the prefixes getopt accepts, -i with its suffix, and the first operand as the script
// when there is no -e or -f. The scripts are one, joined by newlines, as sed joins them.
func parseSedAs(args []string, bsd bool) sedCall {
	var (
		call          sedCall
		scripts, rest []string
	)

	for i := 0; i < len(args); i++ {
		a := args[i]

		switch {
		case a == "--":
			rest = append(rest, args[i+1:]...)
			i = len(args)
		case strings.HasPrefix(a, "--"):
			i = call.long(a, args, i, &scripts)
		case len(a) > 1 && a[0] == '-':
			i = call.cluster(a[1:], args, i, &scripts, bsd)
		default:
			rest = append(rest, a)
		}
	}

	if len(scripts) == 0 && !call.scriptFile && len(rest) > 0 {
		scripts, rest = rest[:1], rest[1:]
	}

	call.files, call.scripts = rest, scripts
	call.writes, call.runs, call.exec = sedScript(strings.Join(scripts, "\n"), bsd)

	return call
}

// long reads one of sed's long options, or a prefix of one, and returns the index of the last word
// it read.
func (c *sedCall) long(a string, args []string, i int, scripts *[]string) int {
	name, value, attached := strings.Cut(a, "=")

	switch {
	case abbreviates(name, "--expression", len("--e")):
		if !attached && i+1 < len(args) {
			i++
			value = args[i]
		}

		*scripts = append(*scripts, value)
	case abbreviates(name, optFile, len("--f")):
		c.scriptFile = true
		i += btoi(!attached)
	case abbreviates(name, "--in-place", len("--i")):
		c.inPlace = true
	case abbreviates(name, "--line-length", len("--l")):
		i += btoi(!attached)
	}

	return i
}

// suffixLike reports whether the word at i reads as a backup suffix of its own: empty, or a dot and
// a name (.bak). GNU's sed takes neither as a script anyone would write.
func suffixLike(args []string, i int) bool {
	return i < len(args) && (args[i] == "" || (strings.HasPrefix(args[i], ".") && !strings.ContainsAny(args[i], "/ ;")))
}

// cluster reads a cluster of sed's short options (-ne, -ni.bak, -Ef): the first of i, e, f, l
// ends it, and what follows is that option's value, or the next word is. BSD's -i takes the next
// word when nothing follows it. It returns the index of the last word it read.
func (c *sedCall) cluster(cluster string, args []string, i int, scripts *[]string, bsd bool) int {
	k := strings.IndexAny(cluster, "eifl")
	if k < 0 {
		return i
	}

	rest := cluster[k+1:]

	switch cluster[k] {
	case 'i':
		c.inPlace = true
		i += btoi(rest == "" && i+1 < len(args) && (bsd || suffixLike(args, i+1)))
	case 'e':
		if rest != "" {
			*scripts = append(*scripts, rest)
		} else if i+1 < len(args) {
			i++
			*scripts = append(*scripts, args[i])
		}
	case 'f':
		c.scriptFile = true
		i += btoi(rest == "")
	case 'l':
		i += btoi(rest == "")
	}

	return i
}

func btoi(b bool) int {
	if b {
		return 1
	}

	return 0
}

// sedScript reads a sed script for the files it writes and the command lines it runs. exec is set
// when it runs what it reads, which the policy cannot judge.
func sedScript(script string, bsd bool) (writes, runs []string, exec bool) {
	for i := sedAddress(script, 0, bsd); i < len(script); i = sedAddress(script, i, bsd) {
		var cmd sedCommand

		cmd, i = sedNext(script, i, bsd)
		if cmd.write != "" {
			writes = append(writes, cmd.write)
		}

		if cmd.run != "" {
			runs = append(runs, cmd.run)
		}

		exec = exec || cmd.exec
	}

	return writes, runs, exec
}

// sedCommand is what one command of a sed script writes or runs, and whether it runs what it read.
type sedCommand struct {
	write, run string
	exec       bool
}

// sedNext reads the command at i, and returns where the next one may start.
func sedNext(script string, i int, bsd bool) (sedCommand, int) {
	var cmd sedCommand

	c := script[i]
	i++

	switch c {
	case 's':
		return sedSubstitute(script, i, bsd)
	case 'y':
		if i < len(script) {
			i = skipDelimited(script, i+1, script[i], 2)
		}
	case 'w', 'W':
		var file string

		file, i = restOfLine(script, i)
		cmd.write = strings.TrimSpace(file)
	case 'e':
		var line string

		line, i = restOfLine(script, i)
		cmd.run = strings.TrimSpace(line)
		cmd.exec = cmd.run == ""
	case '#', 'r', 'R':
		_, i = restOfLine(script, i)
	case 'a', 'i', 'c':
		i = textEnd(script, i)
	case 'b', 't', 'T', ':':
		for i < len(script) && script[i] != ';' && script[i] != '\n' {
			i++
		}
	}

	return cmd, i
}

// sedSubstitute reads s's expression and replacement from their delimiter at i, then its flags:
// the file of w, and whether e runs the line it built.
func sedSubstitute(script string, i int, bsd bool) (sedCommand, int) {
	var cmd sedCommand

	if i >= len(script) {
		return cmd, i
	}

	delim := script[i]
	i = skipDelimited(script, skipRegex(script, i+1, delim, bsd), delim, 1)

	for i < len(script) && strings.IndexByte("gpiImMe0123456789", script[i]) >= 0 {
		cmd.exec = cmd.exec || script[i] == 'e'
		i++
	}

	if i < len(script) && script[i] == 'w' {
		var file string

		file, i = restOfLine(script, i+1)
		cmd.write = strings.TrimSpace(file)
	}

	return cmd, i
}

// sedAddress skips what comes before a command: blanks, separators, braces, line numbers, steps,
// $ and regular expressions (/re/, \cREc, with their I and M flags), and !.
func sedAddress(s string, i int, bsd bool) int {
	for i < len(s) {
		c := s[i]

		switch {
		case strings.IndexByte(" \t\n;{}!,$~+0123456789", c) >= 0:
			i++
		case c == '/' || (c == '\\' && i+1 < len(s)):
			if c == '\\' {
				i++
			}

			i = skipRegex(s, i+1, s[i], bsd)
			for i < len(s) && (s[i] == 'I' || s[i] == 'M') {
				i++
			}
		default:
			return i
		}
	}

	return i
}

// skipRegex skips a regular expression that ends with an unescaped delim, from i. In BSD's sed a
// delim inside a bracket expression ([/], [[:alpha:]/]) does not end it; GNU's ends it there.
func skipRegex(s string, i int, delim byte, bsd bool) int {
	for i < len(s) {
		switch c := s[i]; {
		case c == '\\':
			i += 2
		case c == '[' && bsd && delim != '[':
			i = bracketEnd(s, i+1)
		case c == delim:
			return i + 1
		default:
			i++
		}
	}

	return i
}

// bracketEnd is where a bracket expression that starts at i ends: past its ], which may come first
// (or after ^) as a character, and past the ] of its [:class:], [.coll.] and [=equiv=].
func bracketEnd(s string, i int) int {
	if i < len(s) && s[i] == '^' {
		i++
	}

	if i < len(s) && s[i] == ']' {
		i++
	}

	for i < len(s) {
		switch {
		case s[i] == ']':
			return i + 1
		case s[i] == '[' && i+1 < len(s) && strings.IndexByte(":.=", s[i+1]) >= 0:
			end := strings.Index(s[i+2:], string(s[i+1])+"]")
			if end < 0 {
				return len(s)
			}

			i += 2 + end + 2
		default:
			i++
		}
	}

	return i
}

// skipDelimited skips parts texts that each end with an unescaped delim, from i.
func skipDelimited(s string, i int, delim byte, parts int) int {
	for ; parts > 0 && i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case delim:
			parts--
		}
	}

	return i
}

// restOfLine is the text from i to the end of its line, and where the next line starts.
func restOfLine(s string, i int) (string, int) {
	end := strings.IndexByte(s[i:], '\n')
	if end < 0 {
		return s[i:], len(s)
	}

	return s[i : i+end], i + end + 1
}

// textEnd skips the text of a, i or c: to the end of the line, and on over the lines that end with
// an odd number of backslashes, which continue it.
func textEnd(s string, i int) int {
	for {
		line, next := restOfLine(s, i)

		trailing := len(line) - len(strings.TrimRight(line, "\\"))
		if trailing%2 == 0 || next >= len(s) {
			return next
		}

		i = next
	}
}

var (
	// awkOutput finds the file of print > "file" and printf … >> "file", past the strings printed.
	awkOutput = regexp.MustCompile(`printf?\b(?:[^;{}"]|"[^"]*")*?>>?\s*"([^"]+)"`)
	// awkRedirect finds any output redirection of print or printf: to a file (>, >>) or a command (|).
	awkRedirect = regexp.MustCompile(`printf?\b(?:[^;{}"]|"[^"]*")*?(>|\|)`)
	// awkCommand finds the command line of system("…"), print | "…" and "…" | getline.
	awkCommand = regexp.MustCompile(`system\(\s*"([^"]*)"|\|\s*"([^"]*)"|"([^"]*)"\s*\|\s*getline`)
)

// awkProgram is the program of an awk command: its first operand, unless -f names a file, which
// fromFile says.
func awkProgram(args []string) (program string, fromFile bool) {
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "-f" || strings.HasPrefix(a, "-f") || strings.HasPrefix(a, optFile):
			return "", true
		case a == "-F" || a == "-v":
			i++
		case strings.HasPrefix(a, "-"):
		default:
			return a, false
		}
	}

	return "", false
}

// awkWrites are the files an awk program writes by name.
func awkWrites(args []string) []string {
	var out []string

	program, _ := awkProgram(args)
	for _, m := range awkOutput.FindAllStringSubmatch(program, -1) {
		out = append(out, m[1])
	}

	return out
}

// computed reports whether the shell builds part of a word as the line runs.
func computed(word string) bool {
	return strings.Contains(word, shell.Computed)
}

// recordPlaces are the text of the places the harness keeps what it trusts: git's own files, its
// records, and git's and gh's configuration.
var recordPlaces = []string{".git/", ".cache/harness", "gitconfig", ".config/git", ".config/gh", "sb-review"}

// embedded judges what a sed or awk program does besides printing: an agent's runs no command and
// redirects no print, and the main session's command lines are judged as the lines they are. A
// program that may write (sed's w, awk's >) and names a place the harness trusts, or runs from
// one, is refused to everyone: no parse of it is trusted to know where it writes.
func (j *judge) embedded(name string, args []string) Decision {
	if (name == cmdSed || name == cmdAwk) && j.writesNearRecords(name, args) {
		return deny("this %s program may write where the harness keeps its records or git its files: edit with the editor", name)
	}

	var (
		lines []string
		d     Decision
	)

	switch name {
	case cmdSed:
		lines, d = j.sedLines(args)
	case cmdAwk:
		lines, d = j.awkLines(args)
	}

	if d.Deny {
		return d
	}

	for _, l := range lines {
		if d := j.child(l, maxDepth); d.Deny {
			return d
		}
	}

	return Allow
}

// sedLines are the command lines a sed program runs (its e commands), or why it may not: an agent's
// runs none, and its script is one the policy reads.
func (j *judge) sedLines(args []string) ([]string, Decision) {
	call := parseSed(args)

	switch {
	case j.role.Agent() && (call.scriptFile || slices.ContainsFunc(call.scripts, computed)):
		return nil, deny("write the sed script inline: the policy reads it before it runs")
	case j.role.Agent() && (call.exec || len(call.runs) > 0):
		return nil, deny("sed's e command runs a shell command the policy cannot read")
	}

	return call.runs, Allow
}

// awkLines are the command lines an awk program runs (system, a pipe to or from a command), or why
// it may not: an agent's runs none and writes no file, and its program is one the policy reads.
func (j *judge) awkLines(args []string) ([]string, Decision) {
	program, fromFile := awkProgram(args)

	switch {
	case j.role.Agent() && (fromFile || computed(program)):
		return nil, deny("write the awk program inline: the policy reads it before it runs")
	case j.role.Agent() && awkRedirect.MatchString(program):
		return nil, deny("this awk program writes a file or runs a command: print, and redirect its output in the shell")
	}

	var lines []string
	for _, m := range awkCommand.FindAllStringSubmatch(program, -1) {
		lines = append(lines, m[1]+m[2]+m[3])
	}

	return lines, Allow
}

// writesNearRecords reports whether a sed or awk program may write (a w or W in sed's script, as
// either sed reads it; a print redirected in awk's) while it names a place the harness trusts, or
// the line runs from one.
func (j *judge) writesNearRecords(name string, args []string) bool {
	programs := parseSed(args).scripts
	writes := slices.ContainsFunc(programs, func(s string) bool { return strings.ContainsAny(s, "wW") })

	if name == cmdAwk {
		program, _ := awkProgram(args)
		programs, writes = args, awkRedirect.MatchString(program)
	}

	if !writes {
		return false
	}

	if rel, inside := relative(j.root, j.cwd); inside && (rel == ".git" || under(rel, ".git/") || under(rel, ".cache/")) {
		return true
	}

	return slices.ContainsFunc(programs, func(a string) bool {
		return slices.ContainsFunc(recordPlaces, func(p string) bool { return strings.Contains(a, p) })
	})
}

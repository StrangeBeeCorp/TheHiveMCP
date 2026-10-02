package policy

import (
	"regexp"
	"strings"
)

// sedCall is what a sed command does besides printing: the files it edits in place, those its
// script writes (w, W, the w flag of s), the command lines it runs (e command), and whether it runs
// what it reads (e alone, the e flag of s) or a script the policy has not read (-f).
type sedCall struct {
	inPlace          bool
	files, writes    []string
	runs             []string
	exec, scriptFile bool
}

// parseSed reads sed's arguments, GNU's and BSD's: -e and -f in a cluster or alone, -i with an
// attached suffix (GNU) or the next word as one when it is empty or starts with a dot (BSD's
// sed -i ” …), and the first operand as the script when there is no -e or -f.
func parseSed(args []string) sedCall {
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
		case a == "-i":
			call.inPlace = true
			i += btoi(bsdSuffix(args, i+1))
		case len(a) > 1 && a[0] == '-':
			i = call.cluster(a[1:], args, i, &scripts)
		default:
			rest = append(rest, a)
		}
	}

	if len(scripts) == 0 && !call.scriptFile && len(rest) > 0 {
		scripts, rest = rest[:1], rest[1:]
	}

	call.files = rest

	for _, s := range scripts {
		writes, runs, exec := sedScript(s)
		call.writes, call.runs = append(call.writes, writes...), append(call.runs, runs...)
		call.exec = call.exec || exec
	}

	return call
}

// long reads one of sed's long options, and returns the index of the last word it read.
func (c *sedCall) long(a string, args []string, i int, scripts *[]string) int {
	name, value, attached := strings.Cut(a, "=")

	switch name {
	case "--expression":
		if !attached && i+1 < len(args) {
			i++
			value = args[i]
		}

		*scripts = append(*scripts, value)
	case optFile:
		c.scriptFile = true
		i += btoi(!attached)
	case "--in-place":
		c.inPlace = true
	case "--line-length":
		i += btoi(!attached)
	}

	return i
}

// bsdSuffix reports whether the word at i is the suffix BSD's sed -i takes as its own word: empty,
// or a dot and a name (.bak).
func bsdSuffix(args []string, i int) bool {
	return i < len(args) && (args[i] == "" || (strings.HasPrefix(args[i], ".") && !strings.ContainsAny(args[i], "/ ;")))
}

// cluster reads a cluster of sed's short options (-ne, -ni.bak, -Ef): the first of i, e, f, l
// ends it, and what follows is that option's value, or the next word is. It returns the index of
// the last word it read.
func (c *sedCall) cluster(cluster string, args []string, i int, scripts *[]string) int {
	k := strings.IndexAny(cluster, "eifl")
	if k < 0 {
		return i
	}

	rest := cluster[k+1:]

	switch cluster[k] {
	case 'i':
		c.inPlace = true
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
func sedScript(script string) (writes, runs []string, exec bool) {
	for i := sedAddress(script, 0); i < len(script); i = sedAddress(script, i) {
		var cmd sedCommand

		cmd, i = sedNext(script, i)
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
func sedNext(script string, i int) (sedCommand, int) {
	var cmd sedCommand

	c := script[i]
	i++

	switch c {
	case 's':
		return sedSubstitute(script, i)
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
	case 'a', 'i', 'c', 'r', 'R':
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
func sedSubstitute(script string, i int) (sedCommand, int) {
	var cmd sedCommand

	if i >= len(script) {
		return cmd, i
	}

	i = skipDelimited(script, i+1, script[i], 2)
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
func sedAddress(s string, i int) int {
	for i < len(s) {
		c := s[i]

		switch {
		case strings.IndexByte(" \t\n;{}!,$~+0123456789", c) >= 0:
			i++
		case c == '/' || (c == '\\' && i+1 < len(s)):
			if c == '\\' {
				i++
			}

			i = skipDelimited(s, i+1, s[i], 1)
			for i < len(s) && (s[i] == 'I' || s[i] == 'M') {
				i++
			}
		default:
			return i
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

// textEnd skips the text of a, i or c (and the file of r): to the end of the line, and on over the
// lines a backslash continues.
func textEnd(s string, i int) int {
	for {
		line, next := restOfLine(s, i)
		if !strings.HasSuffix(line, "\\") || next >= len(s) {
			return next
		}

		i = next
	}
}

var (
	// awkOutput finds the file of print > "file" and printf … >> "file", past the strings printed.
	awkOutput = regexp.MustCompile(`printf?\b(?:[^;{}"]|"[^"]*")*?>>?\s*"([^"]+)"`)
	// awkCommand finds the command line of system("…"), print | "…" and "…" | getline.
	awkCommand = regexp.MustCompile(`system\(\s*"([^"]*)"|\|\s*"([^"]*)"|"([^"]*)"\s*\|\s*getline`)
)

// awkProgram is the program of an awk command: its first operand, unless -f names a file.
func awkProgram(args []string) string {
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "-f" || strings.HasPrefix(a, optFile):
			return ""
		case a == "-F" || a == "-v":
			i++
		case strings.HasPrefix(a, "-"):
		default:
			return a
		}
	}

	return ""
}

// awkWrites are the files an awk program writes by name.
func awkWrites(args []string) []string {
	var out []string

	for _, m := range awkOutput.FindAllStringSubmatch(awkProgram(args), -1) {
		out = append(out, m[1])
	}

	return out
}

// embedded judges the command lines a sed or awk program runs: an agent runs none, and the main
// session's are judged as the lines they are.
func (j *judge) embedded(name string, args []string) Decision {
	var lines []string

	switch name {
	case cmdSed:
		call := parseSed(args)
		if j.role.Agent() && (call.exec || len(call.runs) > 0) {
			return deny("sed's e command runs a shell command the policy cannot read")
		}

		lines = call.runs
	case cmdAwk:
		for _, m := range awkCommand.FindAllStringSubmatch(awkProgram(args), -1) {
			lines = append(lines, m[1]+m[2]+m[3])
		}
	}

	for _, l := range lines {
		if d := j.child(l, maxDepth); d.Deny {
			return d
		}
	}

	return Allow
}

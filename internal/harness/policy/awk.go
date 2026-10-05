package policy

import (
	"regexp"
	"slices"
	"strings"
)

var (
	// awkOutput finds the file of print > "file" and printf … >> "file", past the strings printed.
	awkOutput = regexp.MustCompile(`printf?\b(?:[^;{}"]|"[^"]*")*?>>?\s*"([^"]+)"`)
	// awkRedirect finds any output redirection of print or printf: to a file (>, >>) or a command (|).
	awkRedirect = regexp.MustCompile(`printf?\b(?:[^;{}"]|"[^"]*")*?(>|\|)`)
	// awkCommand finds the command line of system("…"), print | "…" and "…" | getline.
	awkCommand = regexp.MustCompile(`system\(\s*"([^"]*)"|\|\s*"([^"]*)"|"([^"]*)"\s*\|\s*getline`)
	// awkIncludes finds gawk's @include and @load, which read code from a file.
	awkIncludes = regexp.MustCompile(`@(include|load)\b`)
)

// awkProgram is the program of an awk command, and whether -f or -E names a file instead.
func awkProgram(args []string) (program string, fromFile bool) {
	call := parseAwk(args)
	return call.program, call.fromFile
}

// awkCall is what an awk command's arguments say: its program (every -e or --source, else its
// first operand, unless -f or -E names a file, which fromFile says), whether gawk's extensions are
// loaded (-i, -l, --include, --load: code the policy cannot read), whether the inplace one is among
// them, the operands after the program, which inplace rewrites, and the options the policy does
// not read (gawk's dump, profile and pretty-print write files): an agent gives none.
type awkCall struct {
	program                  string
	fromFile, loads, inplace bool
	sources, operands        []string
	unread                   []string
}

func parseAwk(args []string) awkCall {
	var (
		call  awkCall
		words []string
	)

	for i := 0; i < len(args); i++ {
		a := args[i]

		switch {
		case a == "--":
			words = append(words, args[i+1:]...)
			i = len(args)
		case strings.HasPrefix(a, "-") && a != "-":
			i += call.option(a, args, i)
		default:
			words = append(words, a)
		}
	}

	switch {
	case len(call.sources) > 0 || call.fromFile:
		call.program, call.operands = strings.Join(call.sources, "\n"), words
	case len(words) > 0:
		call.program, call.operands = words[0], words[1:]
	}

	return call
}

// gawkLong are gawk's long options. It accepts any prefix that names one of them alone.
var gawkLong = strings.Fields(`assign bignum characters-as-bytes copyright csv debug dump-variables exec
field-separator file gen-pot help include lint load no-optimize non-decimal-data optimize persist posix
pretty-print profile re-interval sandbox source traditional use-lc-numeric version`)

// gawkOption is the long option a word names as gawk reads it: whole, or cut to a prefix of one
// option only. "" when it names none or several.
func gawkOption(name string) string {
	name = strings.TrimPrefix(name, "--")
	if slices.Contains(gawkLong, name) {
		return name
	}

	var found []string

	for _, o := range gawkLong {
		if name != "" && strings.HasPrefix(o, name) {
			found = append(found, o)
		}
	}

	if len(found) == 1 {
		return found[0]
	}

	return ""
}

// option reads one of awk's options, and returns how many words after it the option took: a
// short one takes the rest of its word or the next word, a long one its =VALUE or the next word,
// and -W NAME is --NAME, its value attached (-W exec=FILE) or the word after NAME.
func (c *awkCall) option(a string, args []string, i int) int {
	if strings.HasPrefix(a, "--") {
		name, value, attached := strings.Cut(a, "=")
		if !attached {
			value = wordAt(args, i+1)
		}

		return btoi(c.long(name, value) && !attached)
	}

	flag, value := a[1:2], a[2:]
	attached := value != ""

	if !attached {
		value = wordAt(args, i+1)
	}

	switch flag {
	case "f", "E":
		c.fromFile = true
	case "e":
		c.sources = append(c.sources, value)
	case "i", "l":
		c.load(value)
	case "W":
		name, v, has := strings.Cut(value, "=")
		if !has {
			v = wordAt(args, i+2-btoi(attached))
		}

		return btoi(!attached) + btoi(c.long(name, v) && !has)
	case "F", "v":
	default:
		c.unread = append(c.unread, a)
		return 0
	}

	return btoi(!attached)
}

// wordAt is the word at k, or "" past the last one.
func wordAt(args []string, k int) string {
	if k < len(args) {
		return args[k]
	}

	return ""
}

// long reads one of gawk's long options, cut short or not, and reports whether it takes a value.
func (c *awkCall) long(name, value string) bool {
	switch gawkOption(name) {
	case "file", "exec":
		c.fromFile = true
	case cmdSource:
		c.sources = append(c.sources, value)
	case "include", "load":
		c.load(value)
	case "assign", "field-separator":
	default:
		c.unread = append(c.unread, name)
		return false
	}

	return true
}

// load records an extension gawk loads, and whether it is the one that rewrites the operands.
func (c *awkCall) load(name string) {
	c.loads = true
	c.inplace = c.inplace || strings.Contains(name, "inplace")
}

// awkWrites are the files an awk program writes by name.
func awkWrites(args []string) []string {
	var out []string

	call := parseAwk(args)
	for _, m := range awkOutput.FindAllStringSubmatch(call.program, -1) {
		out = append(out, m[1])
	}

	if call.inplace {
		out = append(out, call.operands...)
	}

	return out
}

// awkLines are the command lines an awk program runs (system, a pipe to or from a command), or why
// it may not: an agent's runs none and writes no file, and its program is one the policy reads.
func (j *judge) awkLines(args []string) ([]string, Decision) {
	call := parseAwk(args)
	program, fromFile := call.program, call.fromFile

	switch {
	case j.role.Agent() && call.loads:
		return nil, deny("gawk's -i and -l load code the policy cannot read: write the program inline")
	case j.role.Agent() && len(call.unread) > 0:
		return nil, deny("awk takes only -F, -v and -e from an agent: %s may read or write files the policy does not read", call.unread[0])
	case j.role.Agent() && (fromFile || j.computed(program) || awkIncludes.MatchString(program)):
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

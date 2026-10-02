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
)

// awkProgram is the program of an awk command: its first operand, unless -f names a file, which
// fromFile says.
func awkProgram(args []string) (program string, fromFile bool) {
	call := parseAwk(args)
	return call.program, call.fromFile
}

// awkCall is what an awk command's arguments say: its program (unless -f names a file), whether
// gawk's extensions are loaded (-i, -l, --include, --load: code the policy cannot read), whether
// the inplace one is among them, and the operands after the program, which inplace rewrites.
type awkCall struct {
	program                  string
	fromFile, loads, inplace bool
	operands                 []string
}

func parseAwk(args []string) awkCall {
	var call awkCall

	for i := 0; i < len(args); i++ {
		a := args[i]

		switch {
		case strings.HasPrefix(a, "-"):
			i += call.option(a, args, i)
		case call.program == "" && !call.fromFile:
			call.program = a
		default:
			call.operands = append(call.operands, a)
		}
	}

	return call
}

// option reads one of awk's options, and returns how many words after it the option took.
func (c *awkCall) option(a string, args []string, i int) int {
	switch {
	case a == "-f" || strings.HasPrefix(a, "-f") || strings.HasPrefix(a, optFile):
		c.fromFile = true
	case slices.Contains([]string{"-i", "-l", "--include", "--load"}, a):
		c.loads = true
		c.inplace = c.inplace || (i+1 < len(args) && strings.HasPrefix(args[i+1], "inplace"))

		return 1
	case slices.ContainsFunc([]string{"--include=", "--load=", "-i", "-l"}, func(p string) bool { return strings.HasPrefix(a, p) }):
		c.loads = true
		c.inplace = c.inplace || strings.Contains(a, "inplace")
	case a == "-F" || a == "-v":
		return 1
	}

	return 0
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
	case j.role.Agent() && (fromFile || j.computed(program)):
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

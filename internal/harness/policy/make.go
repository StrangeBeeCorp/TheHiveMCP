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

// evalBuiltin runs its arguments as a line of the same shell.
const evalBuiltin = "eval"

// environment is what a command line may put in the environment of the commands it runs, where
// make reads its variables too: every assignment the line makes (before a command, through env,
// export or declare, or plain, which an exported name or set -a passes on), and the first export
// whose value the policy cannot read (a bare or computed name, set -a), if any.
type environment struct {
	assigns []string
	hidden  string
}

// assigned records a command's assignment words: NAME=value, or an export the policy cannot read.
func (e *environment) assigned(words []string) {
	for _, a := range words {
		switch {
		case strings.Contains(a, "="):
			e.assigns = append(e.assigns, a)
		case e.hidden == "" && a != "" && !strings.HasPrefix(a, "-"):
			e.hidden = "export " + a
		}
	}
}

func (e *environment) add(other environment) {
	e.assigns = append(e.assigns, other.assigns...)

	if e.hidden == "" {
		e.hidden = other.hidden
	}
}

// passedBy is the environment of a parsed line, with that of the lines eval runs, which share
// its shell. A shell's -c script runs in a child, whose exports never reach the line.
func passedBy(line shell.Line, depth int) environment {
	var e environment

	for _, c := range line.Commands {
		e.assigned(c.Assigns)

		if len(c.Words) == 0 {
			continue
		}

		switch path.Base(c.Words[0]) {
		case "set":
			if e.hidden == "" && allexport(c.Words[1:]) {
				e.hidden = "set -a"
			}
		case evalBuiltin:
			if depth < maxDepth {
				e.add(passedBy(shell.Parse(strings.Join(c.Words[1:], " ")), depth+1))
			}
		}
	}

	return e
}

// allexport reports whether set's arguments export every variable set from then on: -a, alone or
// in a cluster (-ea), or -o allexport.
func allexport(args []string) bool {
	for i, a := range args {
		switch {
		case a == "-o" && i+1 < len(args) && args[i+1] == "allexport":
			return true
		case len(a) > 1 && a[0] == '-' && a[1] != '-' && strings.Contains(a[1:], "a"):
			return true
		}
	}

	return false
}

// make judges make: the Makefile expands its variables into shell lines, from its command line and
// from the environment the line gives it.
func (j *judge) make(args []string) Decision {
	for _, a := range append(slices.Clone(args), j.passed.assigns...) {
		if _, value, ok := strings.Cut(a, "="); ok && strings.ContainsAny(value, ";|&$`<>()\n") {
			return deny("make variables are expanded into shell lines: keep their values plain (no ; | & $ ` < > or parentheses)")
		}
	}

	if j.passed.hidden != "" {
		return deny("%s gives make an environment the policy cannot read: pass make's variables on its command line (make gate ID=x)", j.passed.hidden)
	}

	if !j.role.Agent() {
		return Allow
	}

	return j.makeAgent(args)
}

func (j *judge) makeAgent(args []string) Decision {
	has := func(targets ...string) bool {
		return slices.ContainsFunc(args, func(a string) bool { return slices.Contains(targets, a) })
	}

	switch {
	case slices.ContainsFunc(args, makeOption):
		return deny("make runs this repository's Makefile as it is: no other makefile, directory, evaluated text or environment override")
	case j.role.ReadOnly() && !has(subHelp, "help-xml", "test"):
		return deny("the %s reads and reports: only make help and make test", j.role)
	case has("dev-setup", "updatedep"):
		return deny("make dev-setup and make updatedep change the setup or the dependencies: ask the person")
	case j.role.Duel() && has(subPush, "pr"):
		return deny("the %s does not ship: the session that runs the duel does", j.role)
	}

	return j.makeVariables(args)
}

// makeOption reports whether a make option runs something other than this Makefile as it is:
// another makefile (-f, --file, --makefile), another directory (-C, --directory), evaluated text
// (-E, --eval) or the environment over the Makefile (-e). Short options cluster (-sf x.mk) and a
// long one may be cut short (--dir=x), as make accepts them.
func makeOption(a string) bool {
	if name, _, _ := strings.Cut(a, "="); strings.HasPrefix(name, "--") {
		return slices.ContainsFunc([]string{"--file", "--makefile", "--directory", "--eval", "--environment-overrides"},
			func(option string) bool { return abbreviates(name, option, len("--f")) })
	}

	if !strings.HasPrefix(a, "-") {
		return false
	}

	for _, r := range a[1:] {
		switch {
		case strings.ContainsRune("fCEe", r):
			return true
		case strings.ContainsRune("IjlWoO", r):
			// This option takes the rest of the word as its value.
			return false
		}
	}

	return false
}

// makeSets are make's own variables: setting one changes how every recipe runs.
var makeSets = []string{"SHELL", ".SHELLFLAGS", "MAKE", "MAKEFLAGS", "MFLAGS", "GNUMAKEFLAGS", "MAKEFILES", "MAKEFILE_LIST", "MAKESHELL"}

var (
	makeDefinition = regexp.MustCompile(`(?m)^[ \t]*(?:override[ \t]+)?(?:export[ \t]+)?([A-Za-z_.][A-Za-z0-9_.]*)[ \t]*(?:::?=|:::=|\?=|\+=|!=|=)`)
	makeDocumented = regexp.MustCompile(`\b([A-Z][A-Z0-9_]*)=`)
)

// makeVariables refuses an agent the variables that change how the Makefile works: make's own, and
// those the Makefile sets (the pinned images among them) unless make help documents them as the
// target's options (##@ make test-integration [RUN=<regexp>]). A variable the Makefile does not
// know changes nothing.
func (j *judge) makeVariables(args []string) Decision {
	defined, documented := map[string]bool{}, map[string]bool{}

	for _, f := range []string{"Makefile", "scripts/harness.mk"} {
		data, err := os.ReadFile(filepath.Join(j.root, f)) // #nosec G304 -- this checkout's Makefile
		if err != nil {
			continue
		}

		for _, m := range makeDefinition.FindAllStringSubmatch(string(data), -1) {
			defined[m[1]] = true
		}

		for line := range strings.SplitSeq(string(data), "\n") {
			_, help, _ := strings.Cut(line, "##@")
			for _, m := range makeDocumented.FindAllStringSubmatch(help, -1) {
				documented[m[1]] = true
			}
		}
	}

	for _, a := range args {
		name, _, ok := strings.Cut(a, "=")
		name = strings.TrimRight(name, ":?+!")

		switch {
		case !ok || strings.HasPrefix(a, "-"):
		case slices.Contains(makeSets, name):
			return deny("make %s=… changes how every recipe runs: run make with the repository's settings", name)
		case defined[name] && !documented[name]:
			return deny("make %s=… overrides what the Makefile sets: an agent passes only the variables make help documents", name)
		}
	}

	return Allow
}

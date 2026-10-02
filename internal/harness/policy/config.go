package policy

import (
	"slices"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/shell"
)

// configNamesFile is why git and gh may not be pointed at configuration the policy cannot read.
const configNamesFile = "this configuration names a file of configuration the policy cannot read: set the keys one by one"

// gitProgram names, in one section of git's configuration, the keys whose value git runs as a
// command line: directly in the section ("" for any name), and under a subsection
// (diff.<driver>.textconv).
type gitProgram struct {
	top, sub []string
}

// gitPrograms are the keys whose value git runs: an alias, a pager, an editor, a helper, a driver.
var gitPrograms = map[string]gitProgram{
	keyAlias:      {top: []string{""}, sub: []string{""}},
	"core":        {top: []string{keyPager, keyEditor, "sshcommand", "askpass", "fsmonitor", "gitproxy"}},
	"sequence":    {top: []string{keyEditor}},
	keyPager:      {top: []string{""}, sub: []string{""}},
	subDiff:       {top: []string{"external"}, sub: []string{"command", "textconv"}},
	"difftool":    {sub: []string{keyCmd}},
	"mergetool":   {sub: []string{keyCmd}},
	"merge":       {sub: []string{"driver"}},
	"filter":      {sub: []string{"clean", "smudge", "process"}},
	"credential":  {top: []string{"helper"}, sub: []string{"helper"}},
	"gpg":         {top: []string{"program"}, sub: []string{"program"}},
	"uploadpack":  {top: []string{"packobjectshook"}},
	"interactive": {top: []string{"difffilter"}},
	"web":         {top: []string{keyBrowser}},
	keyBrowser:    {sub: []string{keyCmd, "path"}},
	"man":         {sub: []string{keyCmd, "path"}},
	subModule:     {sub: []string{"update"}},
}

// runsAs reports whether git runs the value of key, in any case, as a command line.
func runsAs(key string) bool {
	key = strings.ToLower(key)
	section, rest, _ := strings.Cut(key, ".")
	name := rest[strings.LastIndex(rest, ".")+1:]

	p, ok := gitPrograms[section]
	names := p.top

	if strings.Contains(rest, ".") {
		names = p.sub
	}

	return ok && slices.ContainsFunc(names, func(n string) bool { return n == "" || n == name })
}

// programSection reports whether a section (with or without its subsection) holds keys git runs or
// files it loads.
func programSection(s string) bool {
	section, _, _ := strings.Cut(strings.ToLower(s), ".")

	_, ok := gitPrograms[section]

	return ok || strings.HasPrefix(section, "include")
}

// loadsFile reports whether a key makes git read another file of configuration.
func loadsFile(key string) bool {
	key = strings.ToLower(key)
	return key == "include.path" || (strings.HasPrefix(key, "includeif.") && strings.HasSuffix(key, ".path"))
}

// gitConfig judges git config for every role: a value git will run is judged as the command line
// it is, so an alias or a pager does not carry a command the policy would refuse; a key that
// loads another file, the editor, and a section renamed into one of those are refused. Every key
// followed by another word may be what the call sets, whatever options git reads around them:
// only an option that reads, unsets, lists or renames makes the call set nothing.
func (j *judge) gitConfig(rest []string) Decision {
	call := parseConfig(rest)

	switch call.mode {
	case subEdit:
		return deny("set git's configuration one key at a time (git config KEY VALUE): the policy reads what it sets")
	case renameSection:
		for _, s := range call.operands[min(1, len(call.operands)):] {
			if programSection(s) {
				return deny("a section renamed into %s would make git run values the policy never read: set those keys one by one", s)
			}
		}

		return Allow
	case "", subSet:
	default:
		return Allow
	}

	if slices.ContainsFunc(call.operands, func(o string) bool { return strings.Contains(o, shell.Computed) }) {
		return deny("write git config's key and value out: the policy reads them before git keeps them")
	}

	for k := 0; k+1 < len(call.operands); k++ {
		key, value := call.operands[k], call.operands[k+1]
		if loadsFile(key) {
			return deny(configNamesFile)
		}

		if !runsAs(key) {
			continue
		}

		line := strings.TrimPrefix(value, "!")
		if strings.HasPrefix(strings.ToLower(key), keyAlias+".") && !strings.HasPrefix(value, "!") {
			line = cmdGit + " " + value
		}

		if d := j.child(line, maxDepth); d.Deny {
			return d
		}
	}

	return Allow
}

// renameSection is git config's mode that renames a section.
const renameSection = "rename-section"

// configCall is what a git config command does: its mode ("" or set when it may set a key, edit,
// rename-section, or one that sets nothing) and its operands.
type configCall struct {
	mode     string
	operands []string
}

// configValued are git config's options that take the next word as their value.
var configValued = []string{"-f", optFile, "--blob", "--type", "--default", "--comment", "--value", "--url"}

// configModes are, in git config's old form, the options that make it read, unset, list, rename
// or open the editor instead of setting.
var configModes = []string{
	optGet, "--get-all", "--get-regexp", "--get-urlmatch", "--get-color", "--get-colorbool", "--unset", "--unset-all",
	"--" + renameSection, "--remove-section", "--" + subList, "--" + subEdit,
}

// parseConfig reads git config's arguments, in the old form (git config [--add] KEY VALUE) and the
// new one (git config set KEY VALUE). The editor wins over any other mode.
func parseConfig(rest []string) configCall {
	var call configCall

	if len(rest) > 0 && slices.Contains([]string{subList, "get", subSet, "unset", renameSection, "remove-section", subEdit}, rest[0]) {
		call.mode, rest = rest[0], rest[1:]
	}

	for i := 0; i < len(rest); i++ {
		a := rest[i]

		switch {
		case a == "--":
			call.operands = append(call.operands, rest[i+1:]...)
			return call
		case slices.Contains(configValued, a):
			i++
		case !strings.HasPrefix(a, "-"):
			call.operands = append(call.operands, a)
		case call.mode != subEdit:
			if m := configMode(a); m != "" {
				call.mode = m
			}
		}
	}

	return call
}

// configMode is the mode an option of git config's old form chooses, as git reads it: -l, -e, a
// long option, or a prefix of one that git accepts (--get-r for --get-regexp). It is "" for an
// option that only says how or where to set (--add, --global, --no-type).
func configMode(a string) string {
	switch a {
	case "-l":
		return subList
	case "-e":
		return subEdit
	}

	if !strings.HasPrefix(a, "--") || len(a) < len("--ed") || strings.Contains(a, "=") {
		return ""
	}

	for _, m := range configModes {
		if strings.HasPrefix(m, a) {
			return strings.TrimPrefix(m, "--")
		}
	}

	return ""
}

// ghConfig judges what gh would run later: an alias, the editor, the pager or the browser it is
// configured with. An extension is code a person installs.
func (j *judge) ghConfig(group string, rest []string) (Decision, bool) {
	sub := ""
	if len(rest) > 0 {
		sub = rest[0]
	}

	switch {
	case group == keyAlias && sub == "import":
		return deny(configNamesFile), true
	case group == keyAlias && sub == subSet:
		return j.ghAlias(rest[1:]), true
	case group == subConfig && sub == subSet:
		positional := skipOptions(rest[1:], []string{"-h", "--host"})
		if len(positional) > 1 && slices.Contains([]string{keyEditor, keyPager, keyBrowser}, positional[0]) {
			if strings.Contains(positional[1], shell.Computed) {
				return deny("write gh's %s out: the policy reads it before gh keeps it", positional[0]), true
			}

			return j.child(positional[1], maxDepth), true
		}
	case (group == "extension" || group == "ext" || group == "extensions") &&
		!slices.Contains([]string{subList, "ls", "search", "browse"}, sub):
		return deny("gh extensions are programs a person installs and upgrades"), true
	}

	return Allow, false
}

// ghAlias judges gh alias set NAME EXPANSION: as a shell line with --shell or a leading !, else as
// the gh command it expands to. An expansion read from stdin is not read by the policy.
func (j *judge) ghAlias(args []string) Decision {
	var positional []string

	shellAlias := false

	for _, a := range args {
		switch {
		case a == "-s" || a == "--shell":
			shellAlias = true
		case !strings.HasPrefix(a, "-"):
			positional = append(positional, a)
		}
	}

	if len(positional) < 2 || positional[1] == "-" || strings.Contains(positional[1], shell.Computed) {
		return deny("write the alias's expansion out: the policy reads it before gh keeps it")
	}

	expansion := positional[1]
	if shellAlias || strings.HasPrefix(expansion, "!") {
		return j.child(strings.TrimPrefix(expansion, "!"), maxDepth)
	}

	return j.child("gh "+expansion, maxDepth)
}

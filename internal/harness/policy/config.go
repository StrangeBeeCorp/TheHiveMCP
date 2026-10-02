package policy

import (
	"slices"
	"strings"
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
	keyAlias:      {top: []string{""}},
	"core":        {top: []string{keyPager, keyEditor, "sshcommand", "askpass", "fsmonitor", "gitproxy"}},
	"sequence":    {top: []string{keyEditor}},
	keyPager:      {top: []string{""}},
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
// loads another file, the editor, and a section renamed into one of those are refused.
func (j *judge) gitConfig(rest []string) Decision {
	call := parseConfig(rest)

	switch {
	case call.edit:
		return deny("set git's configuration one key at a time (git config KEY VALUE): the policy reads what it sets")
	case call.renamed != "" && programSection(call.renamed):
		return deny("a section renamed into %s would make git run values the policy never read: set those keys one by one", call.renamed)
	case !call.set:
		return Allow
	case loadsFile(call.key):
		return deny(configNamesFile)
	case !runsAs(call.key):
		return Allow
	}

	line := strings.TrimPrefix(call.value, "!")
	if strings.HasPrefix(strings.ToLower(call.key), "alias.") && !strings.HasPrefix(call.value, "!") {
		line = cmdGit + " " + call.value
	}

	return j.child(line, maxDepth)
}

// configCall is what a git config command asks: a key set to a value, the editor, or a section
// renamed.
type configCall struct {
	key, value, renamed string
	set, edit           bool
}

// configValued are git config's options that take the next word as their value.
var configValued = []string{"-f", optFile, "--blob", "--type", "--default", "--comment", "--value", "--url"}

// parseConfig reads git config's arguments, in the old form (git config [--add] KEY VALUE) and the
// new one (git config set KEY VALUE).
func parseConfig(rest []string) configCall {
	mode := ""
	if len(rest) > 0 && slices.Contains([]string{subList, "get", subSet, "unset", "rename-section", "remove-section", subEdit}, rest[0]) {
		mode, rest = rest[0], rest[1:]
	}

	positional, mode, edit := configWords(rest, mode)
	call := configCall{edit: edit || mode == subEdit}

	switch {
	case (mode == "rename-section" || mode == "--rename-section") && len(positional) > 1:
		call.renamed = positional[1]
	case (mode == "" || mode == subSet) && len(positional) > 1:
		call.set, call.key, call.value = true, positional[0], positional[1]
	}

	return call
}

// configWords sorts git config's words: its operands, the option that says what it does in the old
// form (--get, --unset, --rename-section…), and whether it opens the editor.
func configWords(rest []string, mode string) ([]string, string, bool) {
	var positional []string

	edit := false

	for i := 0; i < len(rest); i++ {
		a := rest[i]

		switch {
		case a == "--":
			return append(positional, rest[i+1:]...), mode, edit
		case slices.Contains(configValued, a):
			i++
		case a == "-e" || a == "--edit":
			edit = true
		case !strings.HasPrefix(a, "-"):
			positional = append(positional, a)
		case mode == "" && !slices.Contains([]string{"--add", "--replace-all"}, a) && !strings.Contains(a, "=") && !scope(a):
			mode = a
		}
	}

	return positional, mode, edit
}

// scope reports whether an option of git config chooses the file or the type, not what it does.
func scope(a string) bool {
	return slices.Contains([]string{"--global", "--system", "--local", "--worktree", "--bool", "--int", "--path",
		"--bool-or-int", "--expiry-date", "--fixed-value", "--show-origin", "--show-scope", "-z", "--null",
		"--name-only", "--includes", "--no-includes", "--all", "--append"}, a)
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

	if len(positional) < 2 || positional[1] == "-" {
		return deny("write the alias's expansion out: the policy reads it before gh keeps it")
	}

	expansion := positional[1]
	if shellAlias || strings.HasPrefix(expansion, "!") {
		return j.child(strings.TrimPrefix(expansion, "!"), maxDepth)
	}

	return j.child("gh "+expansion, maxDepth)
}

package policy

import (
	"slices"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/shell"
)

// configNamesFile is why git and gh may not be pointed at configuration the policy cannot read.
const configNamesFile = "this configuration names a file of configuration the policy cannot read: set the keys one by one"

// loadsFile reports whether a key makes git read another file of configuration.
func loadsFile(key string) bool {
	key = strings.ToLower(key)
	return key == "include.path" || key == "init.templatedir" || (strings.HasPrefix(key, "includeif.") && strings.HasSuffix(key, ".path"))
}

// gitConfig judges git config for every role, without trusting a reading of its options (git
// accepts any prefix of a long one, and one that takes a value may take the next word): every
// word after a key whose value git runs is judged as that value, a key that loads another file
// with any word after it is refused, and so are the editor and a section renamed into one of
// those, wherever their option stands. A computed word is refused: the policy reads what git keeps.
func (j *judge) gitConfig(rest []string) Decision {
	switch {
	case slices.ContainsFunc(rest, func(o string) bool { return strings.Contains(o, shell.Computed) }):
		return deny("write git config's key and value out: the policy reads them before git keeps them")
	case (len(rest) > 0 && rest[0] == subEdit) || slices.ContainsFunc(rest, func(a string) bool { return a == "-e" || cuts(a, "--"+subEdit) }):
		return deny("set git's configuration one key at a time (git config KEY VALUE): the policy reads what it sets")
	}

	for k := range rest {
		if d := j.configWord(rest, k); d.Deny {
			return d
		}
	}

	return Allow
}

// configWord judges the word at k of git config's arguments: a rename is refused (it would carry
// values into a section that runs them), a key that loads a file is refused with any word after
// it, and every word is judged as the command line git may run it as, whatever key it is the
// value of: the table of keys git runs is never complete (trailer.<x>.cmd,
// core.alternateRefsCommand). After an alias key, the words are judged as the git command they
// expand to as well.
func (j *judge) configWord(rest []string, k int) Decision {
	w, after := rest[k], rest[k+1:]

	switch {
	case w == renameSection || cuts(w, "--"+renameSection):
		return deny("renaming a section would carry its values into one that runs them: set the keys one by one")
	case loadsFile(w) && len(after) > 0:
		return deny(configNamesFile)
	}

	if d := j.child(strings.TrimPrefix(w, "!"), maxDepth); d.Deny {
		return d
	}

	if !strings.HasPrefix(strings.ToLower(w), keyAlias+".") {
		return Allow
	}

	for _, value := range after {
		if d := j.child(configLine(w, value), maxDepth); d.Deny {
			return d
		}
	}

	return Allow
}

// cuts reports whether a is option or a prefix of it that git accepts (four characters at least).
func cuts(a, option string) bool {
	return abbreviates(a, option, len("--ed"))
}

// configLine is the command line git runs for a key's value: a shell line for a value that starts
// with !, else for an alias the git command it expands to, and for any other key the value.
func configLine(key, value string) string {
	switch {
	case strings.HasPrefix(value, "!"):
		return strings.TrimPrefix(value, "!")
	case strings.HasPrefix(strings.ToLower(key), keyAlias+"."):
		return cmdGit + " " + value
	}

	return value
}

// renameSection is git config's mode that renames a section.
const renameSection = "rename-section"

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

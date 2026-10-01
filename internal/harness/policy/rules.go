package policy

import (
	"slices"
	"strings"
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

	return Allow
}

// everyone holds for the main session too.
func (j *judge) everyone(name string, args []string) Decision {
	if name == "sudo" || name == "doas" {
		return deny("no sudo here: everything runs as the user, in containers")
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
	case sub == subPush && (j.role.Duel() || j.role.ReadOnly()):
		return deny("the %s does not push: the session that runs the duel ships the change", j.role)
	case j.role.ReadOnly() && !slices.Contains([]string{"select", subList, "check", "hook"}, sub):
		return deny("the %s reads and reports; it does not run gates", j.role)
	}

	return Allow
}

func (j *judge) make(args []string) Decision {
	for _, a := range args {
		if _, value, ok := strings.Cut(a, "="); ok && strings.ContainsAny(value, ";|&$`<>()\n") {
			return deny("make variables are expanded into shell lines: keep their values plain (no ; | & $ ` < > or parentheses)")
		}
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
	case has("-f", "--file", "-C", "--directory", "-E", "--eval", "-e") || slices.ContainsFunc(args, func(a string) bool { return strings.HasPrefix(a, "--eval=") }):
		return deny("make runs this repository's Makefile as it is: no other makefile, directory or evaluated text")
	case j.role.ReadOnly() && !has(subHelp, "help-xml", "test"):
		return deny("the %s reads and reports: only make help and make test", j.role)
	case has("dev-setup", "updatedep"):
		return deny("make dev-setup and make updatedep change the setup or the dependencies: ask the person")
	case j.role.Duel() && has(subPush, "pr"):
		return deny("the %s does not ship: the session that runs the duel does", j.role)
	}

	return Allow
}

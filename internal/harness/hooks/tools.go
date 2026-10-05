package hooks

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
)

// PreToolUse asks the policy about a tool call: edits and reads by path, a shell command only for
// an environment file it names. A denial goes back to the agent with its reason; anything else is
// allowed without a word.
func PreToolUse(root string, in Input) Output {
	role := in.Role()

	var d policy.Decision

	switch in.ToolName {
	case "Bash":
		d = policy.Bash(in.str("command"))
	case "Edit", "Write", "MultiEdit", "NotebookEdit":
		d = policy.Write(role, root, in.str("file_path", "notebook_path"))
	case "Read", "NotebookRead":
		d = policy.Read(role, root, in.str("file_path", "notebook_path"))
	case "Grep", "Glob":
		d = search(role, root, in)
	}

	if in.Unreadable {
		d = policy.Decision{Deny: true, Reason: "harness: this tool call could not be read (its hook input is over 4 MiB, or " +
			"not JSON), so it cannot be judged: split the call, or ask the person to look at the hook"}
	}

	if !d.Deny {
		return Output{}
	}

	return jsonOutput(map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName":            "PreToolUse",
		"permissionDecision":       "deny",
		"permissionDecisionReason": d.Reason,
	}})
}

// search judges a Grep or a Glob by every path it names: its directory, and its file pattern
// (Grep's glob, Glob's pattern), so that `.env*` cannot be searched by pattern either. Searching a
// directory skips the git-ignored files, the environment files among them.
func search(role policy.Role, root string, in Input) policy.Decision {
	for _, key := range []string{"path", "glob", "pattern"} {
		p := in.str(key)
		if p == "" || (key == "pattern" && in.ToolName == "Grep") {
			continue
		}

		if d := policy.Read(role, root, p); d.Deny {
			return d
		}
	}

	return policy.Allow
}

// outside reports whether a path relative to the checkout leaves it: "..", or under it. A name
// that only starts with two dots (..notes.md) is inside.
func outside(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// FileCheck returns what is wrong with one file, for the edit cadence.
type FileCheck func(rel string, content []byte) ([]string, error)

// PostToolUse checks the file an edit wrote, in well under a second. Findings go back to the agent,
// which fixes them before it goes on.
func PostToolUse(root string, in Input, check FileCheck) Output {
	p := in.str("file_path", "notebook_path")
	if p == "" {
		return Output{}
	}

	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}

	rel, err := filepath.Rel(root, p)
	if err != nil || outside(rel) {
		return Output{}
	}

	content, err := os.ReadFile(p) // #nosec G304 -- the file the agent just wrote, inside the checkout
	if err != nil {
		return Output{}
	}

	findings, err := check(filepath.ToSlash(rel), content)
	if err != nil {
		return note("harness: the file check could not run: " + err.Error())
	}

	if len(findings) == 0 {
		return Output{}
	}

	return block(filepath.ToSlash(rel) + " needs fixing before you go on:\n" + strings.Join(findings, "\n"))
}

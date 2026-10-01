package hooks

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
)

// PreToolUse asks the policy about a tool call: shell commands, edits and reads. A denial goes
// back to the agent with its reason; anything else is allowed without a word.
func PreToolUse(root string, in Input) Output {
	role := in.Role()

	var d policy.Decision

	switch in.ToolName {
	case "Bash":
		d = policy.Bash(role, root, cwdIn(root, in.Cwd), in.str("command"))
	case "Edit", "Write", "MultiEdit", "NotebookEdit":
		d = policy.Write(role, root, in.str("file_path", "notebook_path"))
	case "Read", "Grep", "Glob", "NotebookRead":
		if p := in.str("file_path", "notebook_path", "path"); p != "" {
			d = policy.Read(role, root, p)
		}
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

// cwdIn is the shell's directory when it is inside the checkout, else the checkout's root.
func cwdIn(root, cwd string) string {
	if cwd == "" {
		return root
	}

	rel, err := filepath.Rel(root, cwd)
	if err != nil || strings.HasPrefix(rel, "..") {
		return root
	}

	return cwd
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
	if err != nil || strings.HasPrefix(rel, "..") {
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

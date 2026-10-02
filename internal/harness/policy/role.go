// Package policy decides what an agent may do in this repository: which files each role may
// write or read, and which shell commands anyone may run. The Claude Code hooks ask it before
// every tool call (cmd/harness hook pre-tool-use). It allows or denies, with what to do instead;
// it never asks.
//
// It reads a command line as the shell will run it, with the files its paths and patterns name
// now, the files its commands' options name (sed's w, curl -o, find -fprint…), and the command
// lines configuration will run later (a git or gh alias, a pager, an editor); what it cannot read
// there (a computed key, an agent's sed or awk script from a file) it refuses. What a program then
// does by its own logic is beyond it: a script, a recursive search of a directory above a hidden
// one, a value read at run time into a variable the environment already exports, a program it has
// no rule for that writes a file (a copy of the harness under another name among them), a sed or
// awk program of the main session that builds a file name as it runs. The main session's scripts
// and computed paths are its own. The policy keeps an agent
// that follows its instructions in its lane; CI runs every gate again, and the person who merges
// is the last check.
package policy

import (
	"fmt"
	"strings"
)

// Role is who makes a tool call, from the agent type Claude Code sends with it.
type Role int

const (
	// Main is the interactive session: a person is at the keyboard or can be asked.
	Main Role = iota
	// Tester writes failing tests from a spec item, and nothing else (.claude/agents/tester.md).
	Tester
	// Coder makes the tests pass and never edits a test (.claude/agents/coder.md).
	Coder
	// Reviewer reads and reports: the review workers, the verifier, the explore agents.
	Reviewer
	// Other is any other agent: it edits like a developer, within the rules everyone follows.
	Other
)

// RoleOf maps the agent type of a hook call to its role. No agent type is the main session.
func RoleOf(agentType string) Role {
	t := strings.TrimSpace(agentType)

	switch {
	case t == "" || t == mainBranch:
		return Main
	case t == "tester":
		return Tester
	case t == "coder":
		return Coder
	case readOnlyAgent(t):
		return Reviewer
	default:
		return Other
	}
}

func readOnlyAgent(t string) bool {
	switch t {
	case "reviewer", "verifier", "qa", "Explore", "Plan", "claude-code-guide":
		return true
	}

	name := t[strings.LastIndex(t, ":")+1:]

	return strings.HasPrefix(name, "reviewer-") || strings.HasPrefix(name, "rv-")
}

// Agent reports whether the role is an agent rather than the main session.
func (r Role) Agent() bool { return r != Main }

// ReadOnly reports whether the role only reads and reports.
func (r Role) ReadOnly() bool { return r == Reviewer }

// Duel reports whether the role is one of the two sides of the test-first duel.
func (r Role) Duel() bool { return r == Tester || r == Coder }

func (r Role) String() string {
	switch r {
	case Main:
		return "main session"
	case Tester:
		return "Tester"
	case Coder:
		return "Coder"
	case Reviewer:
		return "reviewer"
	case Other:
		return "agent"
	}

	return fmt.Sprintf("role %d", int(r))
}

// Decision is the answer to one tool call. The zero value allows it.
type Decision struct {
	Deny   bool
	Reason string
}

// Allow is the decision that lets a call through.
var Allow = Decision{}

func deny(format string, args ...any) Decision {
	return Decision{Deny: true, Reason: fmt.Sprintf(format, args...)}
}

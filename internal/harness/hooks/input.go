// Package hooks answers the Claude Code hooks of .claude/settings.json, which call
// `bin/harness hook <event>` with the event as JSON on stdin. PreToolUse asks the policy,
// PostToolUse checks the edited file, Stop and SubagentStop run check-fast and refuse to stop red,
// SessionStart tells the agent where it is, PreCompact saves where it was. A hook allows or
// denies with a reason; it never asks.
package hooks

import (
	"encoding/json"
	"fmt"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
)

// Input is what Claude Code sends to a hook. Fields an event does not have stay empty.
type Input struct {
	SessionID      string         `json:"session_id"`
	TranscriptPath string         `json:"transcript_path"`
	Cwd            string         `json:"cwd"`
	Event          string         `json:"hook_event_name"`
	ToolName       string         `json:"tool_name"`
	ToolInput      map[string]any `json:"tool_input"`
	StopHookActive bool           `json:"stop_hook_active"`
	AgentID        string         `json:"agent_id"`
	AgentType      string         `json:"agent_type"`
	Source         string         `json:"source"`
	// Unreadable is true when the input is not the JSON Claude Code sends (too large, or
	// garbled): it is judged as an agent's, and PreToolUse refuses the call.
	Unreadable bool `json:"-"`
}

// Parse decodes a hook's input. One that does not decode is Unreadable: it never passes for the
// main session's, the role with the fewest limits.
func Parse(data []byte) Input {
	var in Input

	if json.Unmarshal(data, &in) != nil {
		return Input{Unreadable: true}
	}

	return in
}

// Role is who made the call; an unreadable input is an agent's.
func (in Input) Role() policy.Role {
	if in.Unreadable {
		return policy.Other
	}

	return policy.RoleOf(in.AgentType)
}

// Actor names the session, or the subagent within it, for per-agent counters.
func (in Input) Actor() string {
	if in.AgentID != "" {
		return in.SessionID + "/" + in.AgentID
	}

	return in.SessionID
}

// str is a string field of the tool input.
func (in Input) str(keys ...string) string {
	for _, k := range keys {
		if s, ok := in.ToolInput[k].(string); ok && s != "" {
			return s
		}
	}

	return ""
}

// Output is what a hook prints; it always exits 0. The zero value says nothing and allows.
type Output struct {
	Stdout string
}

func jsonOutput(v any) Output {
	data, err := json.Marshal(v)
	if err != nil {
		return Output{Stdout: fmt.Sprintf(`{"systemMessage":%q}`, "harness: "+err.Error())}
	}

	return Output{Stdout: string(data)}
}

// note shows a message to the person without stopping anything.
func note(msg string) Output {
	return jsonOutput(map[string]any{"systemMessage": msg})
}

// block sends the reason back to the agent, which keeps working.
func block(reason string) Output {
	return jsonOutput(map[string]any{"decision": "block", "reason": reason})
}

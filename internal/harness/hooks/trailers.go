package hooks

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// The trailers an agent's commit carries (.githooks/prepare-commit-msg, bin/harness trailers), for
// the repository's record of who and what wrote it: its model, its reasoning effort and the skills
// it used. What the session cannot tell is unknown, never guessed.
const (
	TrailerModel  = "Agent-Model"
	TrailerEffort = "Agent-Effort"
	TrailerSkills = "Skills-Used"
	unknownValue  = "unknown"
)

// A transcript entry is read into a buffer of firstLine bytes, grown up to maxTranscriptLine: a
// tool result can be large.
const (
	firstLine         = 1 << 20
	maxTranscriptLine = 16 << 20
)

// Trailers are the trailers of a commit made from a Claude Code session (CLAUDECODE=1, which
// Claude Code sets in its tools' environment), read from that environment and from the session's
// transcript under home: none for a person's commit.
func Trailers(getenv func(string) string, home string) []string {
	if getenv("CLAUDECODE") != "1" {
		return nil
	}

	model, skills := readTranscript(transcriptOf(home, getenv("CLAUDE_CODE_SESSION_ID")))

	effort := getenv("CLAUDE_EFFORT")
	if effort == "" {
		effort = unknownValue
	}

	used := "none"
	if len(skills) > 0 {
		used = strings.Join(skills, ", ")
	}

	return []string{TrailerModel + ": " + model, TrailerEffort + ": " + effort, TrailerSkills + ": " + used}
}

// transcriptOf is the session's newest transcript: Claude Code keeps one per working directory.
func transcriptOf(home, session string) string {
	if session == "" || strings.ContainsAny(session, `/\`) {
		return ""
	}

	found, _ := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", session+".jsonl"))

	newest := ""

	var at int64

	for _, p := range found {
		info, err := os.Stat(p)
		if err == nil && info.ModTime().UnixNano() > at {
			newest, at = p, info.ModTime().UnixNano()
		}
	}

	return newest
}

// readTranscript is the transcript's latest model (unknown without one) and the skills its
// messages are attributed to, sorted.
func readTranscript(p string) (string, []string) {
	model := unknownValue

	f, err := os.Open(p) // #nosec G304 -- the session's own transcript, under the user's home
	if err != nil {
		return model, nil
	}
	defer func() { _ = f.Close() }()

	var skills []string

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, firstLine), maxTranscriptLine)

	for sc.Scan() {
		var entry struct {
			Message struct {
				Model string `json:"model"`
			} `json:"message"`
			Skill string `json:"attributionSkill"`
		}

		if json.Unmarshal(sc.Bytes(), &entry) != nil {
			continue
		}

		if m := entry.Message.Model; m != "" && !strings.HasPrefix(m, "<") {
			model = m
		}

		if entry.Skill != "" && !slices.Contains(skills, entry.Skill) {
			skills = append(skills, entry.Skill)
		}
	}

	slices.Sort(skills)

	return model, skills
}

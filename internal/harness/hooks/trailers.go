package hooks

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// skillsDir holds the skills the repository itself defines: only those are named in a public
// commit. A skill from the user's own plugins is counted, its name left out of the history the
// public mirror publishes.
const skillsDir = ".claude/skills"

// Trailers are the trailers of a commit made from a Claude Code session (CLAUDECODE=1, which
// Claude Code sets in its tools' environment), read from that environment and from the session's
// transcript under home; root is the checkout, whose own skills are named. None for a person's
// commit.
func Trailers(getenv func(string) string, home, root string) []string {
	if getenv("CLAUDECODE") != "1" {
		return nil
	}

	model, skills, err := readTranscript(transcriptOf(home, getenv("CLAUDE_CODE_SESSION_ID")))

	used := skillsUsed(skills, root)
	if err != nil {
		// A transcript read only in part tells nothing for sure.
		model, used = unknownValue, unknownValue
	}

	effort := getenv("CLAUDE_EFFORT")
	if effort == "" {
		effort = unknownValue
	}

	return []string{TrailerModel + ": " + model, TrailerEffort + ": " + effort, TrailerSkills + ": " + used}
}

// skillsUsed names the repository's own skills and counts the others.
func skillsUsed(skills []string, root string) string {
	var named []string

	others := 0

	for _, s := range skills {
		if repoSkill(root, s) {
			named = append(named, s)
		} else {
			others++
		}
	}

	used := strings.Join(named, ", ")

	switch {
	case others > 0 && used == "":
		used = fmt.Sprintf("%d from the user's plugins", others)
	case others > 0:
		used += fmt.Sprintf(" (+%d from the user's plugins)", others)
	case used == "":
		used = "none"
	}

	return used
}

// repoSkill reports whether the checkout defines the skill: a plain name with a SKILL.md under
// skillsDir, never a plugin's "plugin:skill".
func repoSkill(root, skill string) bool {
	if strings.ContainsAny(skill, `:/\`) {
		return false
	}

	_, err := os.Stat(filepath.Join(root, skillsDir, skill, "SKILL.md"))

	return err == nil
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
// messages are attributed to, sorted. No transcript is no error: there is nothing to tell. A
// transcript that cannot be read to its end is one.
func readTranscript(p string) (string, []string, error) {
	model := unknownValue

	f, err := os.Open(p) // #nosec G304 -- the session's own transcript, under the user's home
	if err != nil {
		return model, nil, nil
	}
	defer func() { _ = f.Close() }()

	var skills []string

	// Entries are read whole, however long: a tool result can be megabytes.
	r := bufio.NewReader(f)

	for {
		line, err := r.ReadBytes('\n')

		var entry struct {
			Message struct {
				Model string `json:"model"`
			} `json:"message"`
			Skill string `json:"attributionSkill"`
		}

		if len(line) > 0 && json.Unmarshal(line, &entry) == nil {
			if m := entry.Message.Model; m != "" && !strings.HasPrefix(m, "<") {
				model = m
			}

			if entry.Skill != "" && !slices.Contains(skills, entry.Skill) {
				skills = append(skills, entry.Skill)
			}
		}

		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return model, nil, fmt.Errorf("read the transcript: %w", err)
		}
	}

	slices.Sort(skills)

	return model, skills, nil
}

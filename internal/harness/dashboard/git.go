package dashboard

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Commit is one commit of the history the page reads, with what its trailers say of its author.
type Commit struct {
	Author string
	When   time.Time
	// Model, Effort and Skills are the agent's trailers (Agent-Model, Agent-Effort, Skills-Used),
	// else the model of a Co-Authored-By line; "" for a person's commit.
	Model  string
	Effort string
	Skills []string
}

// Tag is a release tag.
type Tag struct {
	Name string
	When time.Time
}

// errNoGit means git could not read the history.
var errNoGit = errors.New("git cannot read the history")

const (
	fieldSep  = "\x1f"
	recordSep = "\x1e"
)

// coAuthor reads the model from a Co-Authored-By line Claude Code writes, "Claude Opus 4.7 <…>".
var coAuthor = regexp.MustCompile(`(?mi)^Co-Authored-By:\s*(Claude[^<\n]*?)\s*<`)

// skillCount drops the count of the user's own plugin skills a Skills-Used trailer ends with.
var skillCount = regexp.MustCompile(`\s*\(\+\d+ from the user's plugins\)$`)

// history is what git says of the repository.
type history struct {
	Repo, Head string
	Commits    []Commit
	Merges     []time.Time
	Tags       []Tag
}

func readGit(ctx context.Context, o Options) (history, error) {
	git := func(args ...string) (string, error) {
		out, err := runGit(ctx, o.Root, nil, args...)
		if err != nil {
			return "", fmt.Errorf("%w: %w", errNoGit, err)
		}

		return string(out), nil
	}

	var h history

	head, err := git("rev-parse", "--short", "HEAD")
	if err != nil {
		return h, err
	}

	h.Head = strings.TrimSpace(head)

	url, _ := git("remote", "get-url", "origin")
	h.Repo = repoName(strings.TrimSpace(url))

	since := "--since=" + o.Since.Format(time.RFC3339)

	log, err := git("log", o.Main, "HEAD", "--no-merges", since,
		"--format=%an"+fieldSep+"%aI"+fieldSep+"%B"+recordSep)
	if err != nil {
		return h, err
	}

	h.Commits = parseCommits(log)

	first, err := git("log", o.Main, "--first-parent", since, "--format=%cI")
	if err != nil {
		return h, err
	}

	h.Merges = parseTimes(first)

	refs, err := git("tag", "--list", "v*", "--format=%(refname:short)"+fieldSep+"%(creatordate:iso-strict)")
	if err != nil {
		return h, err
	}

	h.Tags = parseTags(refs)

	return h, nil
}

// repoName is owner/name from a remote's URL, or the URL itself.
func repoName(url string) string {
	url = strings.TrimSuffix(url, ".git")

	for _, sep := range []string{"github.com/", "github.com:"} {
		if _, after, ok := strings.Cut(url, sep); ok {
			return after
		}
	}

	if _, after, ok := strings.Cut(url, ":"); ok && !strings.Contains(url, "://") {
		return after
	}

	return url
}

func parseCommits(log string) []Commit {
	var commits []Commit

	for rec := range strings.SplitSeq(log, recordSep) {
		f := strings.SplitN(strings.TrimLeft(rec, "\n"), fieldSep, 3) //nolint:mnd // the format's three fields
		if len(f) != 3 {                                              //nolint:mnd // the format's three fields
			continue
		}

		when, err := time.Parse(time.RFC3339, f[1])
		if err != nil {
			continue
		}

		c := Commit{Author: f[0], When: when}
		trailers(&c, f[2])
		commits = append(commits, c)
	}

	return commits
}

// trailers fills what the message's trailers say of the commit's agent.
func trailers(c *Commit, msg string) {
	for line := range strings.SplitSeq(msg, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		value = strings.TrimSpace(value)

		switch strings.TrimSpace(key) {
		case "Agent-Model":
			c.Model = value
		case "Agent-Effort":
			c.Effort = value
		case "Skills-Used":
			for s := range strings.SplitSeq(skillCount.ReplaceAllString(value, ""), ",") {
				if s = strings.TrimSpace(s); s != "" && s != "none" {
					c.Skills = append(c.Skills, s)
				}
			}
		}
	}

	if c.Model == "" {
		if m := coAuthor.FindStringSubmatch(msg); m != nil {
			c.Model = m[1]
		}
	}

	c.Model = modelName(c.Model)
}

// contextNote drops what a model's name says of its context window: "(1M context)", "[1m]".
var contextNote = regexp.MustCompile(`\s*(\([^)]*\)|\[[^\]]*\])`)

// modelName gives a model one name, whichever way a commit wrote it: the trailer's ID,
// claude-opus-5-5, and Co-Authored-By's name, Claude Opus 5.5 (1M context), are both Claude Opus
// 5.5. A dated ID drops its date.
func modelName(s string) string {
	s = strings.TrimSpace(contextNote.ReplaceAllString(s, ""))
	if s == "" || strings.ContainsAny(s, " ") || !strings.Contains(s, "-") {
		return s
	}

	parts := strings.Split(s, "-")

	var words, version []string

	for _, p := range parts {
		switch {
		case p == "":
		case len(p) == len("20251001") && isDigits(p): // a release date
		case isDigits(p):
			version = append(version, p)
		default:
			words = append(words, strings.ToUpper(p[:1])+p[1:])
		}
	}

	name := strings.Join(words, " ")
	if len(version) > 0 {
		name += " " + strings.Join(version, ".")
	}

	return name
}

func isDigits(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}

func parseTimes(out string) []time.Time {
	var times []time.Time

	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(line))
		if err == nil {
			times = append(times, t)
		}
	}

	return times
}

func parseTags(out string) []Tag {
	var tags []Tag

	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		name, date, ok := strings.Cut(line, fieldSep)
		if !ok {
			continue
		}

		t, err := time.Parse(time.RFC3339, date)
		if err == nil {
			tags = append(tags, Tag{Name: name, When: t})
		}
	}

	return tags
}

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/hooks"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/registry"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"
)

const (
	// maxHookInput bounds what a hook reads from stdin: a tool call, not a file.
	maxHookInput = 4 << 20
	// reportLines bounds the check-fast report an agent gets back.
	reportLines = 120
)

// hook answers one Claude Code hook (.claude/settings.json): the event as JSON on stdin, the
// answer as JSON on stdout. It always exits 0; a denial or a block is in the JSON.
func (e env) hook(ctx context.Context, args []string, stdin io.Reader, out io.Writer) error {
	if len(args) != 1 {
		return usageError{"usage: harness hook pre-tool-use|post-tool-use|stop|subagent-stop|session-start|pre-compact"}
	}

	data, err := io.ReadAll(io.LimitReader(stdin, maxHookInput))
	if err != nil {
		return fmt.Errorf("read the hook input: %w", err)
	}

	in := hooks.Parse(data)

	var o hooks.Output

	switch args[0] {
	case "pre-tool-use":
		hooks.Remember(ctx, e.root, in, e.snapshot)

		o = hooks.PreToolUse(e.root, in)
		if !hooks.Remembered(e.root, in) {
			o = hooks.Deny(hooks.NotRemembered)
		}
	case "post-tool-use":
		o = hooks.PostToolUse(e.root, in, func(rel string, content []byte) ([]string, error) { return e.fileCheck(ctx, rel, content) })
	case "stop", "subagent-stop":
		o = hooks.Stopper{
			Root: e.root, MaxRounds: e.limits.Hooks.MaxRedRounds, Check: e.stopCheck,
			Fingerprint: func(ctx context.Context) (string, error) { return gitx.Fingerprint(ctx, e.root) },
			List:        e.snapshot,
			Crowded:     func(changed, added []string) []string { return e.crowded(ctx, changed, added) },
		}.Stop(ctx, in)
	case "session-start":
		o = hooks.SessionStart(ctx, e.root, in)
	case "pre-compact":
		o = hooks.PreCompact(ctx, e.root)
	default:
		return usageError{fmt.Sprintf("unknown hook %q", args[0])}
	}

	_, _ = io.WriteString(out, o.Stdout)

	return nil
}

// fileCheck runs the edit cadence's built-in gates on one edited file. A file git ignores is not
// checked: it never reaches the repository, which is what the gates judge (BLOCKED.md, scratch
// files, the Tester's sealed tests).
func (e env) fileCheck(ctx context.Context, rel string, content []byte) ([]string, error) {
	if gitx.Ignored(ctx, e.root, rel) {
		return nil, nil
	}

	var out []string

	for _, g := range e.reg.At("edit") {
		if g.Builtin == "" {
			continue
		}

		found, err := checks.OneFile(g.Builtin, rel, content, e.limits)
		if err != nil {
			return nil, fmt.Errorf("gate %s: %w", g.ID, err)
		}

		for _, f := range found {
			out = append(out, f.String())
		}
	}

	return out, nil
}

// stopCheck runs check-fast for the Stop hook: the stop cadence's gates the change selects, or
// only the static ones for the Tester.
func (e env) stopCheck(ctx context.Context, static bool) (hooks.Check, error) {
	var gates []registry.Gate

	for _, g := range e.reg.At("stop") {
		if !static || g.Static {
			gates = append(gates, g)
		}
	}

	var scope scopeFlags

	sel, err := scope.selection(ctx, e)
	if err != nil {
		return hooks.Check{}, err
	}

	e.changed, e.base = sel.Paths, sel.Base

	var buf bytes.Buffer

	rec, err := e.runGates(ctx, "stop", gates, sel, runFlags{}, &buf)
	if err != nil {
		return hooks.Check{}, err
	}

	return hooks.Check{
		Red:        rec.Verdict == runner.Fail,
		Incomplete: rec.Verdict == runner.Incomplete,
		Report:     lastLines(buf.String(), reportLines),
	}, nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = append([]string{fmt.Sprintf("… %d lines above", len(lines)-n)}, lines[len(lines)-n:]...)
	}

	return strings.Join(lines, "\n")
}

// trailers adds an agent's trailers (hooks.Trailers) to a commit message, from .githooks'
// prepare-commit-msg: args are git's, the message file and its source. A person's commit, a merge
// and a squash are left as they are.
func (e env) trailers(ctx context.Context, args []string, _ io.Writer) error {
	if len(args) < 1 {
		return usageError{"usage: harness trailers <commit message file> [source]"}
	}

	if len(args) > 1 && (args[1] == "merge" || args[1] == "squash") {
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("trailers: %w", err)
	}

	add := hooks.Trailers(os.Getenv, home, e.root)
	if len(add) == 0 {
		return nil
	}

	gitArgs := []string{"interpret-trailers", "--in-place", "--if-exists", "replace"}
	for _, t := range add {
		gitArgs = append(gitArgs, "--trailer", t)
	}

	cmd := exec.CommandContext(ctx, "git", append(gitArgs, args[0])...) // #nosec G204 G702 -- git with the trailers and the file git gave
	cmd.Dir = e.root

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("trailers: git interpret-trailers: %w: %s", err, strings.TrimSpace(string(out)))
	}

	return nil
}

// crowded is what an agent's change left near the structure limits (checks.Crowded), as "path:
// why" for its stop; nothing when the checkout cannot be listed.
func (e env) crowded(ctx context.Context, changed, added []string) []string {
	files, err := gitx.Files(ctx, e.root)
	if err != nil {
		return nil
	}

	tree := checks.DirTree{Root: e.root, Names: files}

	found := checks.Crowded(changed, added, files, tree.Read, e.limits.Structure)

	notes := make([]string, 0, len(found))
	for _, f := range found {
		notes = append(notes, f.Path+": "+f.Msg)
	}

	return notes
}

// snapshot is the checkout as it is now, for what an agent changed since its first tool call
// (hooks.Changes). The sealed tests are judged too, though git ignores them.
func (e env) snapshot(ctx context.Context) (hooks.Snapshot, error) {
	files, err := gitx.Contents(ctx, e.root, policy.SealedDir)
	if err != nil {
		return hooks.Snapshot{}, fmt.Errorf("list the checkout's files: %w", err)
	}

	dirty, err := gitx.Dirty(ctx, e.root)
	if err != nil {
		return hooks.Snapshot{}, fmt.Errorf("list the checkout's changes: %w", err)
	}

	sealed, err := gitx.IgnoredUnder(ctx, e.root, policy.SealedDir)
	if err != nil {
		return hooks.Snapshot{}, fmt.Errorf("list the sealed tests: %w", err)
	}

	return hooks.Snapshot{Head: gitx.Head(ctx, e.root), Files: files, Dirty: append(dirty, sealed...)}, nil
}

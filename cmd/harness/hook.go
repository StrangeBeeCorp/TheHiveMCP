package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/hooks"
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
	case "post-tool-use":
		o = hooks.PostToolUse(e.root, in, func(rel string, content []byte) ([]string, error) { return e.fileCheck(ctx, rel, content) })
	case "stop", "subagent-stop":
		o = hooks.Stopper{
			Root: e.root, MaxRounds: e.limits.Hooks.MaxRedRounds, Check: e.stopCheck,
			Fingerprint: func(ctx context.Context) (string, error) { return gitx.Fingerprint(ctx, e.root) },
			List:        e.snapshot,
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

// snapshot is the checkout as it is now, for what an agent changed since its first tool call
// (hooks.Changes).
func (e env) snapshot(ctx context.Context) (hooks.Snapshot, error) {
	files, err := gitx.Contents(ctx, e.root)
	if err != nil {
		return hooks.Snapshot{}, fmt.Errorf("list the checkout's files: %w", err)
	}

	return hooks.Snapshot{Head: gitx.Head(ctx, e.root), Files: files}, nil
}

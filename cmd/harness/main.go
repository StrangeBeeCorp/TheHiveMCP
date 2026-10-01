// Command harness runs the gates of gates/registry.yaml: the checks a change must pass, selected
// by the paths it changed. Run it through bin/harness, which builds it in the pinned Go image.
//
//	harness run --cadence stop|commit|pr|ci|nightly [--base REF | --staged | --all] [--gate ID]... [--force]
//	harness select [--base REF | --staged | --all]
//	harness list [--cadence C]
//	harness check structure|public-safety|docs
//	harness docs [--check]
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gatesdoc"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/registry"
)

const (
	registryPath = "gates/registry.yaml"
	limitsPath   = "gates/limits.yaml"
	exitUsage    = 2
	dockerProbe  = 10 * time.Second
	filePerm     = 0o644
)

// errFailed means the command ran and reported failures; they are already printed.
var errFailed = errors.New("failed")

func main() {
	err := dispatch(context.Background(), os.Args[1:], os.Stdout)

	var usage usageError
	switch {
	case err == nil:
	case errors.Is(err, errFailed):
		os.Exit(1)
	case errors.As(err, &usage):
		_, _ = fmt.Fprintln(os.Stderr, "harness:", err)

		os.Exit(exitUsage)
	default:
		_, _ = fmt.Fprintln(os.Stderr, "harness:", err)

		os.Exit(1)
	}
}

type usageError struct{ msg string }

func (u usageError) Error() string { return u.msg }

// env is what every subcommand needs: the repository root, its registry and its limits.
type env struct {
	root   string
	reg    *registry.Registry
	limits checks.Limits
}

func load(ctx context.Context) (env, error) {
	wd, err := os.Getwd()
	if err != nil {
		return env{}, fmt.Errorf("working directory: %w", err)
	}

	root, err := gitx.Root(ctx, wd)
	if err != nil {
		return env{}, fmt.Errorf("find the repository: %w", err)
	}

	reg, err := registry.Load(filepath.Join(root, registryPath))
	if err != nil {
		return env{}, fmt.Errorf("load the gates: %w", err)
	}

	limits, err := checks.LoadLimits(filepath.Join(root, limitsPath))
	if err != nil {
		return env{}, fmt.Errorf("load the limits: %w", err)
	}

	for _, g := range reg.Gates {
		if g.Builtin != "" && !isBuiltin(g.Builtin) {
			return env{}, fmt.Errorf("gate %q: unknown builtin %q", g.ID, g.Builtin)
		}
	}

	return env{root: root, reg: reg, limits: limits}, nil
}

func dispatch(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return usageError{"usage: harness run|select|list|check|docs (see cmd/harness/main.go)"}
	}

	e, err := load(ctx)
	if err != nil {
		return err
	}

	switch args[0] {
	case "run":
		return e.run(ctx, args[1:], out)
	case "select":
		return e.selectCmd(ctx, args[1:], out)
	case "list":
		return e.list(args[1:], out)
	case "check":
		return e.check(ctx, args[1:], out)
	case "docs":
		return e.docs(args[1:], out)
	default:
		return usageError{fmt.Sprintf("unknown command %q", args[0])}
	}
}

// The checks built into the harness, named by a gate's builtin field.
const (
	builtinStructure = "structure"
	builtinSafety    = "public-safety"
	builtinDocs      = "docs"
)

func isBuiltin(name string) bool {
	switch name {
	case builtinStructure, builtinSafety, builtinDocs:
		return true
	}

	return false
}

// builtin runs a check built into the harness over every file of the repository.
func (e env) builtin(ctx context.Context, name string) ([]string, error) {
	files, err := gitx.Files(ctx, e.root)
	if err != nil {
		return nil, fmt.Errorf("list the files: %w", err)
	}

	var found []checks.Finding

	switch name {
	case builtinStructure:
		found, err = checks.Structure(e.root, files, e.limits.Structure)
	case builtinSafety:
		found, err = checks.PublicSafety(e.root, files, e.limits.PublicSafety)
	case builtinDocs:
		found, err = checks.Docs(e.root, files, e.reg)
	default:
		return nil, fmt.Errorf("unknown builtin %q", name)
	}

	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}

	lines := make([]string, len(found))
	for i, f := range found {
		lines[i] = f.String()
	}

	return lines, nil
}

func dockerUp(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, dockerProbe)
	defer cancel()

	return exec.CommandContext(ctx, "docker", "info").Run() == nil
}

func (e env) check(ctx context.Context, args []string, out io.Writer) error {
	if len(args) != 1 || !isBuiltin(args[0]) {
		return usageError{"usage: harness check structure|public-safety|docs"}
	}

	lines, err := e.builtin(ctx, args[0])
	if err != nil {
		return err
	}

	for _, l := range lines {
		_, _ = fmt.Fprintln(out, l)
	}

	if len(lines) > 0 {
		return errFailed
	}

	_, _ = fmt.Fprintf(out, "%s: clean\n", args[0])

	return nil
}

func (e env) docs(args []string, out io.Writer) error {
	page := gatesdoc.Render(e.reg)

	p := filepath.Join(e.root, gatesdoc.Path)
	if len(args) == 1 && args[0] == "--check" {
		current, err := os.ReadFile(p) // #nosec G304 -- the generated page of this repository
		if err != nil || string(current) != page {
			_, _ = fmt.Fprintf(out, "%s is stale: run make harness-docs\n", gatesdoc.Path)
			return errFailed
		}

		return nil
	}

	err := os.WriteFile(p, []byte(page), filePerm)
	if err != nil {
		return fmt.Errorf("write %s: %w", gatesdoc.Path, err)
	}

	_, _ = fmt.Fprintf(out, "wrote %s\n", gatesdoc.Path)

	return nil
}

func (e env) list(args []string, out io.Writer) error {
	cadence := ""
	if len(args) == 2 && args[0] == "--cadence" {
		cadence = args[1]
	}

	for _, g := range e.reg.Gates {
		if cadence == "" || g.RunsAt(cadence) {
			_, _ = fmt.Fprintf(out, "%-14s %-30s %s\n", g.ID, strings.Join(g.Cadence, ","), strings.Join(g.Suites, ","))
		}
	}

	return nil
}

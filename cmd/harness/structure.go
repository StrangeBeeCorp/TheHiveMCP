package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
)

// defaultTop is how many files and directories the dashboard shows.
const defaultTop = 15

// structure prints the structure gate's dashboard: the largest code files against the line
// limit and the fullest directories against theirs, --top of each, or everything as JSON.
func (e env) structure(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("structure", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	top := fs.Int("top", defaultTop, "how many files and directories to show")
	asJSON := fs.Bool("json", false, "print every code file and directory as JSON")

	err := fs.Parse(args)
	if err != nil || *top < 1 {
		return usageError{"usage: harness structure [--top N] [--json]"}
	}

	tree, err := e.tree(ctx)
	if err != nil {
		return err
	}

	sizes, err := checks.MeasureSizes(tree, e.limits.Structure)
	if err != nil {
		return fmt.Errorf("measure the checkout: %w", err)
	}

	if *asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")

		err = enc.Encode(sizes)
		if err != nil {
			return fmt.Errorf("print the sizes: %w", err)
		}

		return nil
	}

	files, dirs := sizes.Over()
	_, _ = fmt.Fprintf(out, "%d code files over %d lines (each held by its cap in gates/limits.yaml), %d directories over %d code files\n\n",
		files, e.limits.Structure.MaxLines, dirs, e.limits.Structure.MaxCodeFilesPerDir)
	_, _ = fmt.Fprintf(out, "%7s %7s  %s\n", "lines", "left", "code file")

	for _, f := range sizes.Files[:min(*top, len(sizes.Files))] {
		note := ""
		if f.Cap > 0 {
			note = fmt.Sprintf("  (cap %d)", f.Cap)
		}

		_, _ = fmt.Fprintf(out, "%7d %7d  %s%s\n", f.Lines, f.Limit-f.Lines, f.Path, note)
	}

	_, _ = fmt.Fprintf(out, "\n%7s %7s  %s\n", "files", "left", "directory (code files besides tests)")
	for _, d := range sizes.Dirs[:min(*top, len(sizes.Dirs))] {
		_, _ = fmt.Fprintf(out, "%7d %7d  %s\n", d.Files, d.Limit-d.Files, d.Path)
	}

	return nil
}

// Package lint runs the repository's pinned linters and formatters in Docker, on the files the
// harness selects: bin/harness lint and bin/harness fmt, which the lint and fmt gates run.
//
// The package builds each tool's command line as data (Step), so what runs is tested without
// Docker; Runner runs the steps through an Exec, which bin/harness gives the real docker.
package lint

import (
	"fmt"
	"path"
	"slices"
	"strings"
)

// Files are the files of a run, by the tools that read them. A file can be in two lists: a
// workflow is YAML too, and a .mk fragment is a Makefile.
type Files struct {
	Go          []string
	Shell       []string
	Markdown    []string
	YAML        []string
	Workflows   []string
	Dockerfiles []string
	Makefiles   []string
}

// formatExempt are Markdown files that no linter or formatter reads: model-facing resource docs
// whose compact JSON lines and unwrapped tables are what the model is shown.
var formatExempt = []string{"internal/resources/docs/filter-dsl.md"}

// Classify sorts paths, relative to the repository root, by the tools that read them. Git hooks
// and bin/ commands are shell scripts without the extension.
func Classify(paths []string) Files {
	var f Files

	for _, p := range paths {
		for _, list := range []*[]string{byName(&f, p), byBase(&f, path.Base(p))} {
			if list != nil {
				*list = append(*list, p)
			}
		}

		if path.Dir(p) == ".github/workflows" && isYAML(p) {
			f.Workflows = append(f.Workflows, p)
		}
	}

	return f
}

// Formattable are the files a formatter rewrites: Go, shell and Markdown.
func (f Files) Formattable() Files {
	return Files{Go: f.Go, Shell: f.Shell, Markdown: f.Markdown}
}

// Summary counts the files by kind, for the one line a run ends with: "3 go, 1 sh".
func (f Files) Summary() string {
	var parts []string

	for _, k := range []struct {
		n    int
		kind string
	}{
		{len(f.Go), "go"}, {len(f.Shell), "sh"}, {len(f.Markdown), "md"}, {len(f.YAML), "yaml"},
		{len(f.Dockerfiles), "Dockerfile"}, {len(f.Makefiles), "Makefile"},
	} {
		if k.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", k.n, k.kind))
		}
	}

	if len(parts) == 0 {
		return "no files"
	}

	return strings.Join(parts, ", ")
}

// All is every file of the run once, sorted.
func (f Files) All() []string {
	all := slices.Concat(f.Go, f.Shell, f.Markdown, f.YAML, f.Dockerfiles, f.Makefiles)
	slices.Sort(all)

	return slices.Compact(all)
}

// byName is the list a path's extension, or its directory, puts it in.
func byName(f *Files, p string) *[]string {
	switch ext := path.Ext(p); {
	case ext == ".go":
		return &f.Go
	case ext == ".sh" || strings.HasPrefix(p, ".githooks/") || strings.HasPrefix(p, "bin/"):
		return &f.Shell
	case (ext == ".md" || ext == ".mdx") && !slices.Contains(formatExempt, p):
		return &f.Markdown
	case isYAML(p):
		return &f.YAML
	case ext == ".mk":
		return &f.Makefiles
	}

	return nil
}

// byBase is the list a file's base name puts it in: Dockerfile.mcpb, Makefile.local.
func byBase(f *Files, base string) *[]string {
	switch {
	case base == "Dockerfile" || strings.HasPrefix(base, "Dockerfile."):
		return &f.Dockerfiles
	case slices.Contains([]string{"GNUmakefile", "Makefile", "makefile"}, base) || strings.HasPrefix(base, "Makefile."):
		return &f.Makefiles
	}

	return nil
}

func isYAML(p string) bool {
	ext := path.Ext(p)
	return ext == ".yaml" || ext == ".yml"
}

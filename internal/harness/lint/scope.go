package lint

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
)

// Scope is which files a run reads. The zero value is every file the repository has or would
// have on the next git add.
type Scope struct {
	// Staged is the index alone: what the next commit carries.
	Staged bool
	// Changed is the work not committed yet, and the files of Commit when it is set.
	Changed bool
	Commit  string
	// Paths, when set, are the files read, whatever git says.
	Paths []string
}

// Options are what a run of bin/harness lint or fmt asks for.
type Options struct {
	Scope Scope
	// Fix rewrites the files (formatters and the linters' safe fixes); else a run only reports.
	Fix bool
}

// ErrUsage is an argument a run does not understand.
var ErrUsage = errors.New("usage: [--all | --staged | --changed [COMMIT] | PATH...] [--check | --fix]")

// ParseArgs reads the arguments the gates and the make targets pass: one scope (the last wins),
// --check or --fix (fix is the default when fixByDefault), or the paths to read. A word after
// --changed is the commit whose files join the work not committed yet, unless it is a flag.
func ParseArgs(args []string, fixByDefault bool) (Options, error) {
	o := Options{Fix: fixByDefault}

	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--all":
			o.Scope = Scope{}
		case "--staged":
			o.Scope = Scope{Staged: true}
		case "--changed":
			o.Scope = Scope{Changed: true}

			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				o.Scope.Commit = args[i]
			}
		case "--check":
			o.Fix = false
		case "--fix":
			o.Fix = true
		default:
			if strings.HasPrefix(a, "-") {
				return Options{}, fmt.Errorf("%w: unknown flag %q", ErrUsage, a)
			}

			p := filepath.ToSlash(filepath.Clean(a))
			if filepath.IsAbs(a) || p == ".." || strings.HasPrefix(p, "../") {
				return Options{}, fmt.Errorf("%w: %q is not a path inside the repository", ErrUsage, a)
			}

			o.Scope = Scope{Paths: append(o.Scope.Paths, p)}
		}
	}

	return o, nil
}

// Select lists the scope's files that exist in the working tree, relative to root and sorted:
// a deleted file reaches no tool.
func (s Scope) Select(ctx context.Context, root string) ([]string, error) {
	var (
		paths []string
		err   error
	)

	switch {
	case len(s.Paths) > 0:
		paths = s.Paths
	case s.Staged:
		paths, err = gitx.Staged(ctx, root)
	case s.Changed:
		paths, err = s.changed(ctx, root)
	default:
		paths, err = gitx.Files(ctx, root)
	}

	if err != nil {
		return nil, fmt.Errorf("select the files: %w", err)
	}

	existing := make([]string, 0, len(paths))

	for _, p := range paths {
		info, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(p)))
		if statErr == nil && info.Mode().IsRegular() {
			existing = append(existing, p)
		}
	}

	slices.Sort(existing)

	return slices.Compact(existing), nil
}

func (s Scope) changed(ctx context.Context, root string) ([]string, error) {
	paths, err := gitx.Uncommitted(ctx, root)
	if err != nil || s.Commit == "" {
		return paths, err //nolint:wrapcheck // Select wraps it
	}

	if gitx.Resolve(ctx, root, s.Commit) == "" {
		return nil, fmt.Errorf("--changed: %q is not a commit", s.Commit)
	}

	committed, err := gitx.Committed(ctx, root, s.Commit)
	if err != nil {
		return nil, err //nolint:wrapcheck // Select wraps it
	}

	return append(paths, committed...), nil
}

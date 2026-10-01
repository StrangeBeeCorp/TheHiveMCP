package checks

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// Structure checks every file of the repository: a code file stays under max_lines (or under its
// cap, which may only go down), a directory holds at most max_code_files_per_dir code files
// besides its tests, every text file is UTF-8, and the top level holds only the listed entries.
func Structure(root string, files []string, lim StructureLimits) ([]Finding, error) {
	var out []Finding

	perDir := map[string]int{}
	seen := map[string]bool{}

	for _, f := range files {
		content, err := os.ReadFile(filepath.Join(root, f)) //nolint:gosec // G304: files come from git ls-files
		if os.IsNotExist(err) {
			continue // deleted in the working tree
		}

		if err != nil {
			return nil, fmt.Errorf("structure: %w", err)
		}

		seen[f] = true

		out = append(out, fileFindings(f, content, lim)...)
		if matchesFile(lim.CodeFiles, f) && !isBinary(content) && !strings.HasSuffix(f, "_test.go") {
			perDir[path.Dir(f)]++
		}
	}

	out = append(out, dirFindings(perDir, lim.MaxCodeFilesPerDir)...)

	for f := range lim.Caps {
		if !seen[f] {
			out = append(out, Finding{Path: f, Msg: "has a cap in gates/limits.yaml but no longer exists: remove the cap"})
		}
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })

	return out, nil
}

// fileFindings checks one file: its top-level entry, its encoding and, for code, its length.
func fileFindings(f string, content []byte, lim StructureLimits) []Finding {
	var out []Finding

	if top := strings.SplitN(f, "/", 2)[0]; !slices.Contains(lim.Root, top) {
		out = append(out, Finding{Path: f, Msg: fmt.Sprintf("top-level entry %q is not in structure.root of gates/limits.yaml", top)})
	}

	if isBinary(content) {
		return out
	}

	if !validText(content) {
		out = append(out, Finding{Path: f, Msg: "not valid UTF-8"})
	}

	if matchesFile(lim.CodeFiles, f) {
		out = append(out, lineFindings(f, countLines(content), lim)...)
	}

	return out
}

func lineFindings(f string, lines int, lim StructureLimits) []Finding {
	limit, capped := lim.Caps[f]

	switch {
	case !capped && lines > lim.MaxLines:
		return []Finding{{Path: f, Msg: fmt.Sprintf("%d lines, over the limit of %d: split it", lines, lim.MaxLines)}}
	case capped && lines > limit:
		return []Finding{{Path: f, Msg: fmt.Sprintf("%d lines, over its cap of %d: a cap may only go down", lines, limit)}}
	case capped && lines <= lim.MaxLines:
		return []Finding{{Path: f, Msg: fmt.Sprintf("%d lines, within the limit of %d: remove its cap from gates/limits.yaml", lines, lim.MaxLines)}}
	case capped && lines < limit:
		return []Finding{{Path: f, Msg: fmt.Sprintf("%d lines, under its cap of %d: lower the cap to %d in gates/limits.yaml", lines, limit, lines)}}
	}

	return nil
}

func dirFindings(perDir map[string]int, maxFiles int) []Finding {
	var out []Finding

	for dir, n := range perDir {
		if n > maxFiles {
			out = append(out, Finding{Path: dir + "/", Msg: fmt.Sprintf("%d code files besides tests, over the limit of %d: split the package", n, maxFiles)})
		}
	}

	return out
}

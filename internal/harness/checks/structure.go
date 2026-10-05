package checks

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Structure checks every file of the tree: a code file stays under max_lines (or under its cap,
// which may only go down), a directory holds at most max_code_files_per_dir code files besides
// its tests, every text file is UTF-8 with no trailing whitespace and a final newline (verbatim
// files aside), no file is over max_kb, and the top level holds only the listed entries.
func Structure(tree Tree, lim StructureLimits) ([]Finding, error) {
	var out []Finding

	perDir := map[string]int{}
	seen := map[string]bool{}

	for _, f := range tree.Files() {
		content, err := tree.Read(f)
		if errors.Is(err, fs.ErrNotExist) {
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

// fileFindings checks one file: its top-level entry, its size, its encoding and whitespace and,
// for code, its length.
func fileFindings(f string, content []byte, lim StructureLimits) []Finding {
	var out []Finding

	if top := strings.SplitN(f, "/", 2)[0]; !slices.Contains(lim.Root, top) {
		out = append(out, Finding{Path: f, Msg: fmt.Sprintf("top-level entry %q is not in structure.root of gates/limits.yaml", top)})
	}

	out = append(out, cleanFindings(f, content, lim)...)

	if !isBinary(content) && matchesFile(lim.CodeFiles, f) {
		out = append(out, lineFindings(f, countLines(content), lim)...)
	}

	return out
}

// cleanFindings checks what any file must be, alone: under max_kb and, for text, UTF-8 with no
// trailing whitespace and a final newline, unless it is kept verbatim. A YAML file must parse.
func cleanFindings(f string, content []byte, lim StructureLimits) []Finding {
	var out []Finding

	if kb := (len(content) + kibibyte - 1) / kibibyte; lim.MaxKB > 0 && kb > lim.MaxKB {
		out = append(out, Finding{Path: f, Msg: fmt.Sprintf("%d KB, over the limit of %d KB: keep large files out of the repository", kb, lim.MaxKB)})
	}

	if isBinary(content) {
		return out
	}

	if !validText(content) {
		out = append(out, Finding{Path: f, Msg: "not valid UTF-8"})
	}

	err := parseYAML(f, content)
	if err != nil {
		out = append(out, Finding{Path: f, Msg: "not valid YAML: " + err.Error()})
	}

	if matchesFile(lim.Verbatim, f) || len(content) == 0 {
		return out
	}

	if line := trailingSpace(content); line > 0 {
		out = append(out, Finding{Path: f, Line: line, Msg: "trailing whitespace: run make fmt, or remove it"})
	}

	if content[len(content)-1] != '\n' {
		out = append(out, Finding{Path: f, Msg: "does not end with a newline"})
	}

	return out
}

// parseYAML decodes every document of a .yaml or .yml file; any other file is left alone.
func parseYAML(f string, content []byte) error {
	if ext := path.Ext(f); ext != ".yaml" && ext != ".yml" {
		return nil
	}

	dec := yaml.NewDecoder(bytes.NewReader(content))

	for {
		var doc any

		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return err //nolint:wrapcheck // the message is the finding
		}
	}
}

// kibibyte is the unit max_kb counts in.
const kibibyte = 1024

// trailingSpace is the first line that ends in a space or a tab, or 0.
func trailingSpace(content []byte) int {
	for i, line := range bytes.Split(content, []byte{'\n'}) {
		if trimmed := bytes.TrimRight(line, "\r"); len(trimmed) > 0 && (trimmed[len(trimmed)-1] == ' ' || trimmed[len(trimmed)-1] == '\t') {
			return i + 1
		}
	}

	return 0
}

func lineFindings(f string, lines int, lim StructureLimits) []Finding {
	if over := overFindings(f, lines, lim); len(over) > 0 {
		return over
	}

	limit, capped := lim.Caps[f]

	switch {
	case capped && lines <= lim.MaxLines:
		return []Finding{{Path: f, Msg: fmt.Sprintf("%d lines, within the limit of %d: remove its cap from gates/limits.yaml", lines, lim.MaxLines)}}
	case capped && lines < limit:
		return []Finding{{Path: f, Msg: fmt.Sprintf("%d lines, under its cap of %d: lower the cap to %d in gates/limits.yaml", lines, limit, lines)}}
	}

	return nil
}

// overFindings reports a code file longer than the limit, or than its cap.
func overFindings(f string, lines int, lim StructureLimits) []Finding {
	limit, capped := lim.Caps[f]

	switch {
	case !capped && lines > lim.MaxLines:
		return []Finding{{Path: f, Msg: fmt.Sprintf("%d lines, over the limit of %d: split it", lines, lim.MaxLines)}}
	case capped && lines > limit:
		return []Finding{{Path: f, Msg: fmt.Sprintf("%d lines, over its cap of %d: a cap may only go down", lines, limit)}}
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

package checks

import (
	"fmt"
	"path"
	"slices"
	"strings"
)

// nearPercent is how full of its limit a code file is when an agent is asked about it.
const nearPercent = 90

// Crowded lists what an agent's change left close to the structure limits, before a gate refuses
// it: a code file it changed at nearPercent of its limit (or its cap) or more, a directory it added
// a code file to that holds the most code files the limit allows, and a directory it created.
// Splitting is often out of a feature's scope: the agent is asked to split it or to file it as
// debt, not to squeeze. read gives a file's content as it is now; files lists the tree now.
func Crowded(changed, added, files []string, read func(string) ([]byte, error), lim StructureLimits) []Finding {
	var out []Finding

	for _, f := range changed {
		if !matchesFile(lim.CodeFiles, f) {
			continue
		}

		content, err := read(f)
		if err != nil || isBinary(content) {
			continue
		}

		limit := lim.MaxLines
		if c, ok := lim.Caps[f]; ok {
			limit = c
		}

		if lines := countLines(content); limit > 0 && lines*100 >= limit*nearPercent {
			out = append(out, Finding{Path: f, Msg: fmt.Sprintf("%d of %d lines", lines, limit)})
		}
	}

	out = append(out, crowdedDirs(added, files, lim)...)

	return out
}

// crowdedDirs names the directories an agent added code to that are full, and the ones it created.
func crowdedDirs(added, files []string, lim StructureLimits) []Finding {
	perDir := map[string]int{}

	for _, f := range files {
		if matchesFile(lim.CodeFiles, f) && !strings.HasSuffix(f, "_test.go") {
			perDir[path.Dir(f)]++
		}
	}

	var (
		out  []Finding
		seen = map[string]bool{}
	)

	for _, f := range added {
		dir := path.Dir(f)
		if seen[dir] || !matchesFile(lim.CodeFiles, f) {
			continue
		}

		seen[dir] = true

		switch {
		case created(dir, added, files):
			out = append(out, Finding{Path: dir + "/", Msg: "a new directory: is its code out of the parent, and the parent left clean?"})
		case lim.MaxCodeFilesPerDir > 0 && perDir[dir] >= lim.MaxCodeFilesPerDir:
			out = append(out, Finding{Path: dir + "/", Msg: fmt.Sprintf("%d of %d code files", perDir[dir], lim.MaxCodeFilesPerDir)})
		}
	}

	return out
}

// created reports whether every file of dir is one the agent added: the directory is new.
func created(dir string, added, files []string) bool {
	for _, f := range files {
		if path.Dir(f) == dir && !slices.Contains(added, f) {
			return false
		}
	}

	return true
}

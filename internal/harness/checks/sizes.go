package checks

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// FileSize is a code file's length against the line limit, and its cap when it has one.
type FileSize struct {
	Path  string `json:"path"`
	Lines int    `json:"lines"`
	Limit int    `json:"limit"`
	Cap   int    `json:"cap,omitempty"`
}

// DirSize is how many code files a directory holds besides its tests, against the limit.
type DirSize struct {
	Path  string `json:"path"`
	Files int    `json:"files"`
	Limit int    `json:"limit"`
}

// Sizes is the structure gate's dashboard: every code file and every directory that holds code,
// the closest to its limit (or the furthest over it) first.
type Sizes struct {
	Files []FileSize `json:"files"`
	Dirs  []DirSize  `json:"dirs"`
}

// MeasureSizes measures the code files and the directories of the tree as the structure gate
// counts them: lines of each code file, and code files besides tests per directory.
func MeasureSizes(tree Tree, lim StructureLimits) (Sizes, error) {
	var s Sizes

	perDir := map[string]int{}

	for _, f := range tree.Files() {
		if !matchesFile(lim.CodeFiles, f) {
			continue
		}

		content, err := tree.Read(f)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}

		if err != nil {
			return Sizes{}, fmt.Errorf("measure %s: %w", f, err)
		}

		if isBinary(content) {
			continue
		}

		s.Files = append(s.Files, FileSize{Path: f, Lines: countLines(content), Limit: lim.MaxLines, Cap: lim.Caps[f]})

		if !strings.HasSuffix(f, "_test.go") {
			perDir[path.Dir(f)]++
		}
	}

	for dir, n := range perDir {
		s.Dirs = append(s.Dirs, DirSize{Path: dir + "/", Files: n, Limit: lim.MaxCodeFilesPerDir})
	}

	sort.SliceStable(s.Files, func(i, j int) bool {
		if s.Files[i].Lines != s.Files[j].Lines {
			return s.Files[i].Lines > s.Files[j].Lines
		}

		return s.Files[i].Path < s.Files[j].Path
	})

	sort.SliceStable(s.Dirs, func(i, j int) bool {
		if s.Dirs[i].Files != s.Dirs[j].Files {
			return s.Dirs[i].Files > s.Dirs[j].Files
		}

		return s.Dirs[i].Path < s.Dirs[j].Path
	})

	return s, nil
}

// Over counts the files over the line limit (each held by its cap) and the directories over theirs.
func (s Sizes) Over() (files, dirs int) {
	for _, f := range s.Files {
		if f.Lines > f.Limit {
			files++
		}
	}

	for _, d := range s.Dirs {
		if d.Files > d.Limit {
			dirs++
		}
	}

	return files, dirs
}

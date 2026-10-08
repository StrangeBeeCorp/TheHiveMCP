package change

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
)

func lines(n int) []byte {
	return []byte(strings.Repeat("x\n", n))
}

func files(m map[string]int) func(string) ([]byte, bool) {
	return func(p string) ([]byte, bool) {
		n, ok := m[p]
		if !ok {
			return nil, false
		}

		return lines(n), true
	}
}

// A file the change lengthens is listed when it ends three quarters of the way to its limit (its
// cap, else max_lines), the closest first; a file it shortens, a short one, and a file the
// structure gate does not measure are not.
func TestMovesListTheFilesTheChangeBringsNearTheirLimit(t *testing.T) {
	t.Parallel()

	const capped, near, added = "capped.go", "near.go", "new.go"

	lim := checks.StructureLimits{MaxLines: 100, CodeFiles: []string{"*.go"}, Caps: map[string]int{capped: 200}}
	// Each file's length on the base and on the branch (-1: absent), and whether the change adds to it.
	sizes := []struct {
		path          string
		before, after int
		grows         bool
	}{
		{near, 70, 80, true}, {added, -1, 75, true}, {capped, 150, 190, true}, {"short.go", 10, 74, true},
		{"shrunk.go", 99, 90, true}, {"doc.md", 1, 400, true}, {"gone.go", 50, -1, true}, {"same.go", 99, 99, false},
		{"origin.go", 60, -1, false}, {movedGo, -1, 75, true},
	}

	d, was, now := Diff{}, map[string]int{}, map[string]int{}
	for _, s := range sizes {
		d[s.path] = nil
		if s.grows {
			d[s.path] = []Line{{N: 1}}
		}

		if s.before >= 0 {
			was[s.path] = s.before
		}

		if s.after >= 0 {
			now[s.path] = s.after
		}
	}

	assert.Equal(t, []Move{
		{Path: capped, Before: 150, After: 190, Limit: 200},
		{Path: near, Before: 70, After: 80, Limit: 100},
		{Path: movedGo, Before: 60, After: 75, Limit: 100},
		{Path: added, Before: 0, After: 75, Limit: 100, New: true},
	}, Moves(d, Renames{movedGo: "origin.go"}, lim, files(was), files(now)))
}

// markedGo is a Go file with markers in comments, in strings, and on a line the change leaves.
const markedGo = "package a\n\n" +
	"var x = 1 //nolint:mnd // why\n" + // 3
	"//nolint\n" + // 4
	"var s = \"//nolint:x // TODO: in a string\"\n" + // 5
	"// TODO: later\n" + // 6
	"var r = `\n// TODO: in a raw string\n`\n" + // 7-9
	"/* a block\nTODO: inside it */\n" + // 10-11
	"// a TODO it only mentions is not one\n" + // 12
	"// TODO: not added\n" // 13

// In a Go file, the //nolint directives and the comments that start with TODO on the added lines
// count, not a comment that only mentions the word, a string, or a line the change leaves; in
// another code file, a comment opener followed by TODO counts; a file that is not code, or a Go
// file gone at HEAD, counts nothing.
func TestMarkCountsTheNolintAndTODOTheChangeAdds(t *testing.T) {
	t.Parallel()

	var goLines []Line
	for n := 3; n <= 12; n++ {
		goLines = append(goLines, Line{N: n})
	}

	d := Diff{
		aGoFile:    goLines,
		"gone.go":  {{N: 1}},
		"run.sh":   {{Text: "# TODO fix"}, {Text: "echo //nolint # mentions a TODO"}},
		"notes.md": {{Text: "// TODO"}},
	}
	code := func(f string) bool { return f != "notes.md" }
	read := func(f string) ([]byte, bool) { return []byte(markedGo), f == aGoFile }

	assert.Equal(t, Markers{Nolint: 2, TODO: 3}, Mark(d, code, read))
}

// The dependencies added are the modules the branch requires and the base does not: a version
// bump is not one.
func TestNewDependenciesAreTheModulesTheBaseDidNotRequire(t *testing.T) {
	t.Parallel()

	base := []byte("module m\n\nrequire a.com/x v1.0.0\n\nrequire (\n\tb.com/y v1.0.0 // indirect\n)\n")
	branch := []byte("module m\n\nrequire a.com/x v1.1.0\n\nrequire (\n\t// a comment\n\tb.com/y v1.0.0 // indirect\n\tc.com/z v0.2.0\n" +
		"\td.com/w v0.1.0 // indirect\n)\n\nreplace e.com/v => ../v\n")

	assert.Equal(t, []string{"c.com/z v0.2.0", "d.com/w v0.1.0 // indirect"}, NewDependencies(base, branch))
	assert.Empty(t, NewDependencies(nil, nil))
}

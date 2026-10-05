package checks_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
)

const (
	cleanGo   = "package a\n"
	sizesRoot = "pkg"
	hugeGo    = "pkg/huge.go"
	goPattern = "*.go"
)

func mem(files map[string]string) checks.MemTree {
	tree := checks.MemTree{}
	for name, content := range files {
		tree[name] = []byte(content)
	}

	return tree
}

// Every text file is clean: no trailing whitespace, a final newline, under max_kb, and a YAML
// file parses, every document of it. A verbatim
// file (a canary patch, whose blank context lines are a space) keeps its bytes; a binary file is
// judged only by its size. The edit check reports the same for the one file edited.
func TestTextFilesAreCleanAndSmall(t *testing.T) {
	t.Parallel()

	lim := checks.StructureLimits{MaxLines: 500, MaxKB: 1, Root: []string{sizesRoot, "gates"}, Verbatim: []string{"gates/canaries/*.patch"}}
	tree := mem(map[string]string{
		"pkg/ok.go":              cleanGo,
		"pkg/spaces.md":          "# title\nsome text  \nmore\n",
		"pkg/tab.yaml":           "a: 1\t\n",
		"pkg/crlf.txt":           "a\r\nb \r\n",
		"pkg/no-newline.txt":     "the end",
		"pkg/big.json":           strings.Repeat("a", 1100) + "\n",
		"pkg/big.bin":            "\x00" + strings.Repeat("a", 2000),
		"gates/canaries/x.patch": "@@ -1 +1 @@\n \n-a\n+b",
		"pkg/empty.txt":          "",
		"pkg/plain.md":           "a\nb\n",
		"pkg/ok.yaml":            "a: 1\n---\nb: [2]\n",
		"pkg/bad.yml":            "a: 1\n---\nb: [2\n",
	})

	found, err := checks.Structure(tree, lim)
	require.NoError(t, err)
	require.NotEmpty(t, found)
	assert.Equal(t, "pkg/bad.yml", found[0].Path, "the second document does not parse")
	assert.True(t, strings.HasPrefix(found[0].Msg, "not valid YAML: "), found[0].Msg)
	assert.Equal(t, strings.Join([]string{
		"pkg/big.bin: 2 KB, over the limit of 1 KB: keep large files out of the repository",
		"pkg/big.json: 2 KB, over the limit of 1 KB: keep large files out of the repository",
		"pkg/crlf.txt:2: trailing whitespace: run make fmt, or remove it",
		"pkg/no-newline.txt: does not end with a newline",
		"pkg/spaces.md:2: trailing whitespace: run make fmt, or remove it",
		"pkg/tab.yaml:1: trailing whitespace: run make fmt, or remove it",
	}, "\n"), messages(found[1:]))

	edit, err := checks.OneFile("structure", "pkg/spaces.md", tree["pkg/spaces.md"], checks.Limits{Structure: lim})
	require.NoError(t, err)
	assert.Equal(t, "pkg/spaces.md:2: trailing whitespace: run make fmt, or remove it", messages(edit))
}

// The dashboard lists every code file by length, the furthest over the limit first with its cap,
// and every directory by its code files besides tests; Over counts what only a cap holds.
func TestSizesRankWhatIsClosestToItsLimit(t *testing.T) {
	t.Parallel()

	lim := checks.StructureLimits{
		MaxLines: 10, MaxCodeFilesPerDir: 2, CodeFiles: []string{goPattern},
		Caps: map[string]int{hugeGo: 30},
	}
	tree := mem(map[string]string{
		hugeGo:          lines(30),
		"pkg/a.go":      lines(4),
		"pkg/b.go":      lines(9),
		"pkg/b_test.go": lines(12),
		"other/c.go":    lines(2),
		"pkg/notes.md":  lines(50),
		"pkg/blob.go":   "\x00binary",
	})

	s, err := checks.MeasureSizes(tree, lim)
	require.NoError(t, err)

	require.Len(t, s.Files, 5, "code files only, binaries left out")
	assert.Equal(t, checks.FileSize{Path: hugeGo, Lines: 30, Limit: 10, Cap: 30}, s.Files[0])
	assert.Equal(t, "pkg/b_test.go", s.Files[1].Path)
	assert.Equal(t, []checks.DirSize{{Path: "pkg/", Files: 3, Limit: 2}, {Path: "other/", Files: 1, Limit: 2}}, s.Dirs,
		"a directory counts its code files besides tests")

	files, dirs := s.Over()
	assert.Equal(t, 2, files, "huge.go held by its cap, and the long test")
	assert.Equal(t, 1, dirs)
}

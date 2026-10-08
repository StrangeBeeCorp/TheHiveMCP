package checks_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
)

func paths(found []checks.Finding) []string {
	out := make([]string, 0, len(found))
	for _, f := range found {
		out = append(out, f.Path+": "+f.Msg)
	}

	return out
}

// A page under docs/ sits in one of Diataxis's four folders, or in evaluation/ (ADR-0001's
// published evidence), or is the index; anywhere else is refused.
func TestAPageSitsInADiataxisFolder(t *testing.T) {
	t.Parallel()

	tree := checks.MemTree{
		"docs/README.md":                 []byte("# Docs\n"),
		"docs/tutorial/first.md":         []byte("# First\n"),
		"docs/how-to/do.md":              []byte("# Do\n"),
		"docs/reference/tools/search.md": []byte("# Search\n"),
		"docs/explanation/adr/0001.md":   []byte("# ADR\n"),
		"docs/evaluation/v1/summary.md":  []byte("# Summary\n"),
		"docs/notes.md":                  []byte("# Notes\n"),
		"docs/guides/setup.md":           []byte("# Setup\n"),
	}

	found, err := checks.DocsLayout(tree)
	require.NoError(t, err)
	require.Len(t, found, 2, strings.Join(paths(found), "\n"))
	assert.Equal(t, "docs/guides/setup.md", found[0].Path)
	assert.Equal(t, "docs/notes.md", found[1].Path)
	assert.Contains(t, found[0].Msg, "Diataxis")
	assert.Contains(t, found[0].Msg, "docs/evaluation", "the refusal names every folder the gate accepts")
}

// A long page needs section headings to be searched by; a heading inside a code block does not
// count, and a short page needs none.
func TestALongPageHasSectionHeadings(t *testing.T) {
	t.Parallel()

	lines := func(n int) string { return strings.Repeat("text\n", n) }
	tree := checks.MemTree{
		"docs/how-to/flat.md":      []byte("# Flat\n" + lines(129)),
		"docs/how-to/fenced.md":    []byte("# Fenced\n## One\n```\n## not a heading\n```\n" + lines(130)),
		"docs/how-to/tilde.md":     []byte("# Tilde\n## One\n~~~\n## not a heading\n~~~\n" + lines(130)),
		"docs/how-to/sectioned.md": []byte("# Sectioned\n## One\n" + lines(130) + "## Two\n"),
		"docs/how-to/at-limit.md":  []byte("# At the limit\n" + lines(119)),
		"docs/how-to/over.md":      []byte("# Over\n" + lines(120)),
		"docs/how-to/short.md":     []byte("# Short\nA few lines.\n"),
	}

	found, err := checks.DocsLayout(tree)
	require.NoError(t, err)
	require.Len(t, found, 4, strings.Join(paths(found), "\n"))

	got := make([]string, 0, len(found))
	for _, f := range found {
		got = append(got, f.Path)
	}

	assert.ElementsMatch(t, []string{"docs/how-to/fenced.md", "docs/how-to/flat.md", "docs/how-to/over.md", "docs/how-to/tilde.md"}, got,
		"120 lines is the limit, 121 is over it, and a heading in a fence of either kind does not count")
	assert.Contains(t, found[0].Msg, "## headings")
}

// A relative link in docs/ or .github/ reaches a file of the repository: from the page's folder,
// or from the root with a leading slash; an anchor or a link out of the repository is not checked.
func TestARelativeLinkReachesAFile(t *testing.T) {
	t.Parallel()

	tree := checks.MemTree{
		"RELEASING.md":            []byte("# Releasing\n"),
		"docs/images/demo.gif":    []byte("GIF"),
		"docs/how-to/handle a.md": []byte("# Handle\n"),
		"docs/how-to/b.md": []byte("# B\n[ok](../../RELEASING.md) [img](../images/demo.gif) [anchor](#b) [space](handle%20a.md#x)\n" +
			"[web](https://example.com/x.md) [mail](mailto:a@b.c) [root](/RELEASING.md)\n" +
			"[gone](../reference/gone.md)\n```\n[fenced](nowhere.md)\n```\n" +
			"write it as `[text](example.md)` [titled](missing-titled.md \"A title\") [angle](<missing angle.md#x>) [ok-angle](<handle a.md>)\n"),
		".github/SECURITY.md": []byte("# Security\n[cve](../docs/how-to/missing.md)\n"),
	}

	found, err := checks.DocsLayout(tree)
	require.NoError(t, err)
	assert.Equal(t, []string{
		".github/SECURITY.md:2: links to ../docs/how-to/missing.md, which is not in the repository",
		"docs/how-to/b.md:4: links to ../reference/gone.md, which is not in the repository",
		"docs/how-to/b.md:8: links to missing-titled.md, which is not in the repository",
		"docs/how-to/b.md:8: links to missing angle.md, which is not in the repository",
	}, paths(found), "a link in a code span is an example; a titled or angle-bracketed link is checked")
}

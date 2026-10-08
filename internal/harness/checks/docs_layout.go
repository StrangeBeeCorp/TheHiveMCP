package checks

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"regexp"
	"strings"
)

// docsFolders are where a page under docs/ may sit: Diataxis's four kinds, and evaluation/, the
// published evidence ADR-0001 keeps in one folder the README links. docs/README.md is the index.
var docsFolders = []string{"tutorial", "how-to", "reference", "explanation", "evaluation"}

const (
	docsIndex = "docs/README.md"
	// longPage is how many lines a page holds before it needs section headings to be searched by.
	longPage = 120
	// minSections is how many ## headings a long page needs.
	minSections = 2
)

// mdLink is a markdown link's target: [text](target), the anchor apart.
var mdLink = regexp.MustCompile(`\]\(([^)\s#]*)(#[^)\s]*)?\)`)

// DocsLayout checks the documentation's shape: every page under docs/ sits in a Diataxis folder, a
// long page has section headings, and a relative link in docs/ or .github/ reaches a file.
func DocsLayout(tree Tree) ([]Finding, error) {
	var out []Finding

	for _, f := range tree.Files() {
		if !strings.HasSuffix(f, ".md") || (!strings.HasPrefix(f, "docs/") && !strings.HasPrefix(f, ".github/")) {
			continue
		}

		content, err := tree.Read(f)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}

		if err != nil {
			return nil, fmt.Errorf("docs layout: %w", err)
		}

		if strings.HasPrefix(f, "docs/") {
			out = append(out, placed(f)...)
			out = append(out, sectioned(f, content)...)
		}

		out = append(out, brokenLinks(tree, f, content)...)
	}

	return out, nil
}

// placed refuses a page outside the documentation's folders.
func placed(f string) []Finding {
	if f == docsIndex {
		return nil
	}

	rest := strings.TrimPrefix(f, "docs/")

	for _, folder := range docsFolders {
		if strings.HasPrefix(rest, folder+"/") {
			return nil
		}
	}

	return []Finding{{Path: f, Msg: "outside the documentation's folders: move it to docs/tutorial, docs/how-to, docs/reference or docs/explanation (Diataxis)"}}
}

// sectioned refuses a long page with too few section headings to search by.
func sectioned(f string, content []byte) []Finding {
	lines, sections := 0, 0
	fenced := false

	sc := bufio.NewScanner(bytes.NewReader(content))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20) //nolint:mnd // a markdown line is short

	for sc.Scan() {
		lines++

		t := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			fenced = !fenced

			continue
		}

		if !fenced && strings.HasPrefix(t, "## ") {
			sections++
		}
	}

	if lines > longPage && sections < minSections {
		return []Finding{{Path: f, Msg: fmt.Sprintf("%d lines and %d section heading(s): a page over %d lines needs at least %d ## headings to be searched by", lines, sections, longPage, minSections)}}
	}

	return nil
}

// brokenLinks finds the relative links of a page whose target is not in the tree.
func brokenLinks(tree Tree, f string, content []byte) []Finding {
	var out []Finding

	fenced := false

	sc := bufio.NewScanner(bytes.NewReader(content))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20) //nolint:mnd // a markdown line is short

	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		if t := strings.TrimSpace(text); strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			fenced = !fenced

			continue
		}

		if fenced {
			continue
		}

		for _, m := range mdLink.FindAllStringSubmatch(text, -1) {
			target := m[1]
			if target == "" || strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
				continue // an anchor in the page, or a link out of the repository
			}

			if !tree.Exists(linkedFile(f, target)) {
				out = append(out, Finding{Path: fmt.Sprintf("%s:%d", f, line), Msg: "links to " + target + ", which is not in the repository"})
			}
		}
	}

	return out
}

// linkedFile is the repository path a relative link target of page f points at.
func linkedFile(f, target string) string {
	p, err := url.PathUnescape(target)
	if err != nil {
		p = target
	}

	if strings.HasPrefix(p, "/") {
		return strings.TrimPrefix(path.Clean(p), "/")
	}

	return path.Clean(path.Join(path.Dir(f), p))
}

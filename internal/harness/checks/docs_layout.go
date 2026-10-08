package checks

import (
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

// mdLink is a markdown link's target: [text](target), [text](<target>) or [text](target "title"),
// the anchor apart. Group 1 is an angle-bracketed target (anchor included), group 2 a bare one.
var mdLink = regexp.MustCompile(`\]\(\s*(?:<([^>]*)>|([^)\s#<]*))(?:#[^)\s]*)?(?:\s+"[^"]*")?\s*\)`)

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

	return []Finding{{Path: f, Msg: "outside the documentation's folders: move it to docs/" + strings.Join(docsFolders, ", docs/") +
		" (Diataxis's four kinds, and evaluation/ for published evidence)"}}
}

// markdownLine is a line of a page, and whether it sits in a fenced code block.
type markdownLine struct {
	text   string
	fenced bool
}

// readLines splits a page into lines, however long, each marked in or out of a fenced code block;
// a fence line itself counts as fenced.
func readLines(content []byte) []markdownLine {
	var out []markdownLine

	fenced := false

	for text := range strings.SplitSeq(string(content), "\n") {
		t := strings.TrimSpace(text)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			fenced = !fenced
			out = append(out, markdownLine{text: text, fenced: true})

			continue
		}

		out = append(out, markdownLine{text: text, fenced: fenced})
	}

	if n := len(out); n > 0 && out[n-1].text == "" {
		out = out[:n-1] // the newline that ends the last line
	}

	return out
}

// sectioned refuses a long page with too few section headings to search by.
func sectioned(f string, content []byte) []Finding {
	lines := readLines(content)
	sections := 0

	for _, l := range lines {
		if !l.fenced && strings.HasPrefix(strings.TrimSpace(l.text), "## ") {
			sections++
		}
	}

	if len(lines) > longPage && sections < minSections {
		return []Finding{{Path: f, Msg: fmt.Sprintf("%d lines and %d section heading(s): a page over %d lines needs at least %d ## headings to be searched by", len(lines), sections, longPage, minSections)}}
	}

	return nil
}

// brokenLinks finds the relative links of a page whose target is not in the tree. A link in a
// fenced block or a code span is an example, not a link.
func brokenLinks(tree Tree, f string, content []byte) []Finding {
	var out []Finding

	for i, l := range readLines(content) {
		if l.fenced {
			continue
		}

		for _, m := range mdLink.FindAllStringSubmatch(inlineCode.ReplaceAllString(l.text, ""), -1) {
			target := m[2]
			if m[1] != "" {
				target, _, _ = strings.Cut(m[1], "#")
			}

			if target == "" || strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
				continue // an anchor in the page, or a link out of the repository
			}

			if !tree.Exists(linkedFile(f, target)) {
				out = append(out, Finding{Path: fmt.Sprintf("%s:%d", f, i+1), Msg: "links to " + target + ", which is not in the repository"})
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

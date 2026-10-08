package change

import (
	"go/scanner"
	"go/token"
	"regexp"
	"strings"
)

// Markers counts the //nolint directives and the TODO comments the change adds to code files.
type Markers struct {
	Nolint int
	TODO   int
}

var (
	// nolintRe is a //nolint directive: a comment that starts with the word, then a colon, a
	// space or its end.
	nolintRe = regexp.MustCompile(`^//nolint(?:[:\s]|$)`)
	// todoWord is a TODO in a comment's text.
	todoWord = regexp.MustCompile(`\bTODO\b`)
	// todoRe is a TODO after a comment opener on a line of a file that is not Go.
	todoRe = regexp.MustCompile(`(?://|#|/\*).*\bTODO\b`)
)

// Mark counts the markers in the lines the change adds to the files code accepts. In a Go file,
// read at HEAD, only its comments count, as the Go scanner finds them: a "// TODO" inside a
// string is not one. Another code file has no parser here, so a TODO after a comment opener on an
// added line counts.
func Mark(d Diff, code func(string) bool, read func(string) ([]byte, bool)) Markers {
	var m Markers

	for file, added := range d {
		switch {
		case !code(file) || len(added) == 0:
		case strings.HasSuffix(file, ".go"):
			content, ok := read(file)
			if ok {
				m = m.plus(markGo(added, goComments(content)))
			}
		default:
			m = m.plus(markText(added))
		}
	}

	return m
}

func (m Markers) plus(o Markers) Markers {
	return Markers{Nolint: m.Nolint + o.Nolint, TODO: m.TODO + o.TODO}
}

// markGo counts the markers in the comments of the added lines of a Go file.
func markGo(added []Line, comments map[int][]string) Markers {
	var m Markers

	for _, l := range added {
		for _, text := range comments[l.N] {
			if nolintRe.MatchString(text) {
				m.Nolint++
			}

			if todoWord.MatchString(text) {
				m.TODO++
			}
		}
	}

	return m
}

// markText counts the TODOs after a comment opener on the added lines of a file that is not Go.
func markText(added []Line) Markers {
	var m Markers

	for _, l := range added {
		if todoRe.MatchString(l.Text) {
			m.TODO++
		}
	}

	return m
}

// goComments is the text of each comment of a Go file, by line: a comment over several lines
// gives each of its lines. The scanner goes on past a syntax error.
func goComments(src []byte) map[int][]string {
	byLine := map[int][]string{}
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))

	var s scanner.Scanner
	s.Init(file, src, nil, scanner.ScanComments)

	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			return byLine
		}

		if tok != token.COMMENT {
			continue
		}

		line := fset.Position(pos).Line
		for i, part := range strings.Split(lit, "\n") {
			byLine[line+i] = append(byLine[line+i], part)
		}
	}
}

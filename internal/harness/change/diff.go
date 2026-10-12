// Package change measures a pull request's own change, on its base and on its branch: the share
// of the changed statements the tests ran, the files it brings closer to their size limit, and the
// //nolint, TODO and dependencies it adds. bin/harness evidence --markdown prints it with the Gate
// evidence.
package change

import (
	"bufio"
	"strconv"
	"strings"
)

// Line is a line the change adds: its number in the new file, and its text.
type Line struct {
	N    int
	Text string
}

// Diff is the lines a change adds, by the path of the file at HEAD. A deleted file has none, and
// a file moved as it was has none either.
type Diff map[string][]Line

// Renames maps the path of a file the change moves, at HEAD, to its path on the base.
type Renames map[string]string

// ParseDiff reads a patch written with no context lines and with renames detected (git diff -U0
// -M): the lines it adds, and the files it moves.
func ParseDiff(patch string) (Diff, Renames) {
	d, renames := Diff{}, Renames{}

	var (
		file, from string
		next, left int
	)

	scanner := bufio.NewScanner(strings.NewReader(patch))
	scanner.Buffer(nil, len(patch)+1)

	for scanner.Scan() {
		text := scanner.Text()

		switch {
		case left > 0 && strings.HasPrefix(text, "+"):
			// Inside a hunk, a line that starts with "+++ " is still an added line.
			d[file] = append(d[file], Line{N: next, Text: text[1:]})
			next++
			left--
		case strings.HasPrefix(text, "rename from "):
			from = unquote(strings.TrimPrefix(text, "rename from "))
		case strings.HasPrefix(text, "rename to ") && from != "":
			renames[unquote(strings.TrimPrefix(text, "rename to "))] = from
			from = ""
		case strings.HasPrefix(text, "+++ "):
			file = newPath(strings.TrimPrefix(text, "+++ "))
		case strings.HasPrefix(text, "@@ ") && file != "":
			next, left = hunk(text)
		}
	}

	return d, renames
}

// newPath is the path a "+++" header names, without its "b/", or "" for /dev/null.
func newPath(header string) string {
	p, ok := strings.CutPrefix(unquote(header), "b/")
	if !ok {
		return ""
	}

	return p
}

// unquote is a path as git writes it in a header: quoted when it holds a tab, a quote or a
// newline, and followed by a tab in a "+++" line when it holds a space.
func unquote(p string) string {
	p = strings.TrimSuffix(p, "\t")

	unquoted, err := strconv.Unquote(p)
	if err == nil {
		return unquoted
	}

	return p
}

// hunk is where a hunk's new lines start and how many it adds: "@@ -a,b +c,d @@", where a
// missing ",d" means one line.
func hunk(header string) (start, count int) {
	fields := strings.Fields(header)

	const newField = 2
	if len(fields) <= newField || !strings.HasPrefix(fields[newField], "+") {
		return 0, 0
	}

	from, n, found := strings.Cut(fields[newField][1:], ",")

	start, err := strconv.Atoi(from)
	if err != nil {
		return 0, 0
	}

	count = 1
	if found {
		count, err = strconv.Atoi(n)
		if err != nil {
			return 0, 0
		}
	}

	return start, count
}

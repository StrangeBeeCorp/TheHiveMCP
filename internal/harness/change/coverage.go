package change

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Block is a block of statements a coverage profile measures: lines Start to End of File, a path
// in the module.
type Block struct {
	File       string
	Start, End int
	Statements int
	Covered    bool
}

// ErrProfile means the profile is not what go test -coverprofile writes.
var ErrProfile = errors.New("not a coverage profile")

// ParseProfile reads a coverage profile, with the module's prefix removed from its paths. A block
// that appears more than once (several test binaries) is one block, covered if any run covered it.
func ParseProfile(profile []byte, module string) ([]Block, error) {
	index := map[string]int{}

	var blocks []Block

	scanner := bufio.NewScanner(bytes.NewReader(profile))

	for line := 1; scanner.Scan(); line++ {
		text := scanner.Text()
		if line == 1 && strings.HasPrefix(text, "mode:") || text == "" {
			continue
		}

		b, err := parseBlock(text, module)
		if err != nil {
			return nil, fmt.Errorf("%w: line %d: %q", ErrProfile, line, text)
		}

		key, _, _ := strings.Cut(text, " ")
		if i, seen := index[key]; seen {
			blocks[i].Covered = blocks[i].Covered || b.Covered
			continue
		}

		index[key] = len(blocks)
		blocks = append(blocks, b)
	}

	err := scanner.Err()
	if err != nil {
		return nil, fmt.Errorf("read the profile: %w", err)
	}

	return blocks, nil
}

// parseBlock reads "pkg/file.go:10.20,12.3 2 1": the block's lines and columns, its statements,
// and how many times it ran.
func parseBlock(text, module string) (Block, error) {
	fields := strings.Fields(text)

	const fieldCount = 3
	if len(fields) != fieldCount {
		return Block{}, ErrProfile
	}

	file, span, ok := strings.Cut(fields[0], ":")
	from, to, ok2 := strings.Cut(span, ",")
	start, err1 := strconv.Atoi(lineOf(from))
	end, err2 := strconv.Atoi(lineOf(to))
	stmts, err3 := strconv.Atoi(fields[1])
	count, err4 := strconv.Atoi(fields[2])

	if !ok || !ok2 || errors.Join(err1, err2, err3, err4) != nil {
		return Block{}, ErrProfile
	}

	return Block{File: strings.TrimPrefix(file, module+"/"), Start: start, End: end, Statements: stmts, Covered: count > 0}, nil
}

// lineOf is the line of a "line.column" position.
func lineOf(pos string) string {
	line, _, _ := strings.Cut(pos, ".")
	return line
}

// Coverage is how much of a change's Go code the unit tests ran: the statements of the blocks a
// changed line falls in. Unmeasured are the changed Go files with no statement in the profile
// (only declarations, or a package no test builds).
type Coverage struct {
	Statements int
	Covered    int
	Unmeasured []string
}

// Percent is the share of the changed statements the tests ran.
func (c Coverage) Percent() float64 {
	if c.Statements == 0 {
		return 0
	}

	return hundred * float64(c.Covered) / float64(c.Statements)
}

const hundred = 100

// DiffCoverage measures the change's non-test Go files against the blocks of a profile. A block
// counts once, however many of its lines changed.
func DiffCoverage(d Diff, blocks []Block) Coverage {
	byFile := map[string][]Block{}
	for _, b := range blocks {
		byFile[b.File] = append(byFile[b.File], b)
	}

	var c Coverage

	for file, added := range d {
		if !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") || len(added) == 0 {
			continue
		}

		fileBlocks, measured := byFile[file]
		if !measured {
			c.Unmeasured = append(c.Unmeasured, file)
			continue
		}

		for _, b := range fileBlocks {
			if !touches(b, added) {
				continue
			}

			c.Statements += b.Statements
			if b.Covered {
				c.Covered += b.Statements
			}
		}
	}

	sort.Strings(c.Unmeasured)

	return c
}

// touches reports whether one of the added lines falls in the block.
func touches(b Block, added []Line) bool {
	for _, l := range added {
		if l.N >= b.Start && l.N <= b.End {
			return true
		}
	}

	return false
}

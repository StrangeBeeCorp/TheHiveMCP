package change

import (
	"cmp"
	"slices"
	"sort"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
)

// Move is a code file the change brings closer to its size limit: its length on the base (at its
// path there, for a file it moves) and on the branch, against its cap or max_lines. New is a file
// the base did not have.
type Move struct {
	Path          string
	Before, After int
	Limit         int
	New           bool
}

// nearShare is how close to its limit, in quarters, a file that grows must come to be listed:
// three quarters of the way.
const (
	nearShare = 3
	quarters  = 4
)

// Moves lists the files the change makes longer that end at least three quarters of the way to
// their limit, the closest first. before and after read a file on the base and on the branch; a
// file the change moves is read on the base at its old path.
func Moves(d Diff, renames Renames, lim checks.StructureLimits, before, after func(string) ([]byte, bool)) []Move {
	var moves []Move

	for file, added := range d {
		content, ok := after(file)
		if !ok || len(added) == 0 {
			continue
		}

		lines, limit, code := lim.LineLimit(file, content)
		if !code || limit <= 0 || lines*quarters < limit*nearShare {
			continue
		}

		was := 0

		old, existed := before(cmp.Or(renames[file], file))
		if existed {
			was, _, _ = lim.LineLimit(file, old)
		}

		if lines > was {
			moves = append(moves, Move{Path: file, Before: was, After: lines, Limit: limit, New: !existed})
		}
	}

	sort.Slice(moves, func(i, j int) bool {
		a, b := moves[i].After*moves[j].Limit, moves[j].After*moves[i].Limit
		if a != b {
			return a > b
		}

		return moves[i].Path < moves[j].Path
	})

	return moves
}

// Requires lists the modules a go.mod requires, with their versions, "// indirect" kept.
func Requires(gomod []byte) map[string]string {
	reqs := map[string]string{}
	inBlock := false

	for line := range strings.SplitSeq(string(gomod), "\n") {
		line = strings.TrimSpace(line)

		switch {
		case line == "require (":
			inBlock = true
			continue
		case inBlock && line == ")":
			inBlock = false
			continue
		case strings.HasPrefix(line, "require "):
			line = strings.TrimPrefix(line, "require ")
		case !inBlock:
			continue
		}

		fields := strings.Fields(line)

		const modAndVersion = 2
		if len(fields) >= modAndVersion && !strings.HasPrefix(fields[0], "//") {
			reqs[fields[0]] = strings.Join(fields[1:], " ")
		}
	}

	return reqs
}

// NewDependencies lists the modules the branch's go.mod requires and the base's does not, each
// with its version, sorted.
func NewDependencies(base, branch []byte) []string {
	was := Requires(base)

	var added []string

	for mod, version := range Requires(branch) {
		if _, ok := was[mod]; !ok {
			added = append(added, mod+" "+version)
		}
	}

	slices.Sort(added)

	return added
}

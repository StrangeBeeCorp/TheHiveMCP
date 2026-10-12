package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
)

// ticketKey is a Jira key: it names the sealed directory, so nothing else may.
var ticketKey = regexp.MustCompile(`^[A-Z][A-Z0-9]*-[0-9]+$`)

// errUnseal means a sealed test cannot go where it says.
var errUnseal = errors.New("unseal")

// move is one sealed test and where it goes in the checkout.
type move struct{ from, to string }

// unseal moves the Tester's sealed tests of a ticket, .claude/sealed/<ticket>/<path>, to <path>
// in the checkout: the end of the duel, when the Coder may see them. Nothing moves unless all can:
// each must be a test, none may replace a file, and a move that fails puts the ones before it back.
func (e env) unseal(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("unseal", flag.ContinueOnError)
	ticket := fs.String("ticket", "", "the ticket whose sealed tests to move into the code (DL-1234)")

	err := fs.Parse(args)
	if err != nil {
		return usageError{err.Error()}
	}

	if !ticketKey.MatchString(*ticket) {
		return usageError{"unseal needs --ticket, a ticket key such as DL-1234"}
	}

	dir := filepath.Join(e.root, policy.SealedDir, *ticket)

	moves, err := unsealPlan(dir, e.root)
	if err != nil {
		return err
	}

	var moved []move

	for _, m := range moves {
		err = os.MkdirAll(filepath.Dir(m.to), dirPerm)
		if err == nil {
			err = os.Rename(m.from, m.to)
		}

		if err != nil {
			return fmt.Errorf("move %s: %w%s", m.from, err, putBack(moved))
		}

		moved = append(moved, m)
	}

	for _, m := range moves {
		rel, _ := filepath.Rel(e.root, m.to)
		_, _ = fmt.Fprintln(out, "unsealed", rel)
	}

	_ = os.RemoveAll(dir)
	_, _ = fmt.Fprintf(out, "%d sealed test file(s) of %s are in the code: run them\n", len(moves), *ticket)

	return nil
}

// unsealPlan lists the moves out of a ticket's sealed directory, refusing any that would not land
// on a new test file of the checkout.
func unsealPlan(dir, root string) ([]move, error) {
	var moves []move

	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return fmt.Errorf("sealed path %s: %w", p, err)
		}

		rel = filepath.ToSlash(rel)
		to := filepath.Join(root, filepath.FromSlash(rel))

		class, protected := policy.ClassOf(rel)

		switch {
		case !policy.IsTest(rel):
			return fmt.Errorf("%w: %s is not a test file (*_test.go, testdata/, internal/testutils/)", errUnseal, rel)
		case protected:
			return fmt.Errorf("%w: %s is %s, which the Tester may not change", errUnseal, rel, class)
		case exists(to):
			return fmt.Errorf("%w: %s already exists: a sealed test is a new file, give it its own name", errUnseal, rel)
		}

		moves = append(moves, move{from: p, to: to})

		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s has no sealed tests", errUnseal, dir)
	}

	if err != nil {
		return nil, fmt.Errorf("plan the moves: %w", err)
	}

	sort.Slice(moves, func(i, j int) bool { return moves[i].to < moves[j].to })

	return moves, nil
}

// putBack moves the sealed tests already in the code back under the sealed directory, the last
// first, and names those it could not.
func putBack(moved []move) string {
	var stuck []string

	for _, m := range slices.Backward(moved) {
		if os.Rename(m.to, m.from) != nil {
			stuck = append(stuck, m.to)
		}
	}

	if len(stuck) == 0 {
		return ""
	}

	return "; these stayed in the code: " + strings.Join(stuck, ", ")
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

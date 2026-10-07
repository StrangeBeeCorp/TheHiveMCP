package checks_test

import (
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
)

const (
	nearGo   = "pkg/near.go"
	cappedGo = "pkg/capped.go"
	notesMD  = "pkg/notes.md"
)

// What an agent's change left close to the limits: a code file at 90 % of its limit or of its
// cap, a full directory it added code to, a directory it created. A file well under its limit, a
// doc, or a test file added to a full directory (tests do not count) is no finding.
func TestCrowdedNamesWhatTheChangeLeftNearTheLimits(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		nearGo:            lines(450),
		"pkg/fine.go":     lines(100),
		cappedGo:          lines(560),
		notesMD:           strings.Repeat("x\n", 600),
		"full/a.go":       lines(10),
		"full/b.go":       lines(10),
		"full/c.go":       lines(10),
		"full/c_test.go":  lines(10),
		"fresh/new.go":    lines(10),
		"fresh/sub/x.go":  lines(10),
		"roomy/a.go":      lines(10),
		"roomy/b_test.go": lines(10),
	}
	read := func(f string) ([]byte, error) {
		if c, ok := files[f]; ok {
			return []byte(c), nil
		}

		return nil, fs.ErrNotExist
	}
	all := make([]string, 0, len(files))

	for f := range files {
		all = append(all, f)
	}

	lim := checks.StructureLimits{MaxLines: 500, MaxCodeFilesPerDir: 3, CodeFiles: []string{goPattern}, Caps: map[string]int{cappedGo: 600}}

	got := checks.Crowded(
		[]string{nearGo, "pkg/fine.go", cappedGo, notesMD, "pkg/gone.go"},
		[]string{"full/c.go", "full/c_test.go", "fresh/new.go", "roomy/b_test.go"},
		all, read, lim)

	assert.Equal(t, []checks.Finding{
		{Path: nearGo, Msg: "450 of 500 lines"},
		{Path: cappedGo, Msg: "560 of 600 lines"},
		{Path: "full/", Msg: "3 of 3 code files"},
		{Path: "fresh/", Msg: "a new directory: is its code out of the parent, and the parent left clean?"},
	}, got)

	assert.Empty(t, checks.Crowded(nil, nil, all, func(string) ([]byte, error) { return nil, errors.New("unread") }, lim), "nothing changed")
}

package issues_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/issues"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/triage"
)

const branch = "feat/DL-1-a-feature"

func debt(path string) issues.Debt {
	return issues.Debt{Title: "split the search handler", Path: path, Why: "470 of 500 lines; the split is not this feature's.",
		Branch: branch, Head: "0123456789abcdef"}
}

// A debt issue says what, where, why, and the change it was found in.
func TestADebtIssueSaysWhatWhereWhyAndWhereItWasFound(t *testing.T) {
	t.Parallel()

	d := debt("internal/tools/search/handler.go")
	assert.Equal(t, "debt: split the search handler", d.IssueTitle())

	body := d.Body()
	assert.Contains(t, body, "**Path:** `internal/tools/search/handler.go`")
	assert.Contains(t, body, "**Why:** 470 of 500 lines; the split is not this feature's.")
	assert.Contains(t, body, "**Found on:** `feat/DL-1-a-feature` at `0123456`.")
	assert.Contains(t, body, "make next")
}

// A path is filed once: an open debt issue about it is found by its path line, not a path that only
// starts the same.
func TestAPathAlreadyFiledIsFound(t *testing.T) {
	t.Parallel()

	open := []triage.Issue{
		{Number: 3, Title: "debt: x", Body: debt("internal/tools/search/handler.go").Body()},
		{Number: 4, Title: "nightly: lint fails (checks)", Body: "the lint log"},
	}

	found, ok := issues.Filed(open, "internal/tools/search/handler.go")
	assert.True(t, ok)
	assert.Equal(t, 3, found.Number)

	_, ok = issues.Filed(open, "internal/tools/search")
	assert.False(t, ok, "the directory is another path")
}

// The issues a branch filed are counted by its branch line, for the cap.
func TestTheDebtABranchFiledIsCounted(t *testing.T) {
	t.Parallel()

	other := debt("a.go")
	other.Branch = "feat/DL-2-another"
	open := []triage.Issue{{Body: debt("a.go").Body()}, {Body: debt("b.go").Body()}, {Body: other.Body()}}

	assert.Equal(t, 2, issues.FiledFrom(open, branch))
	assert.Equal(t, 1, issues.FiledFrom(open, "feat/DL-2-another"))
	assert.Equal(t, 0, issues.FiledFrom(open, "feat/DL-3"))
}

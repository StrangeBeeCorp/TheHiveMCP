package issues

import (
	"fmt"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/triage"
)

const (
	// DebtLabel marks the work an agent or a person found out of their change's scope.
	DebtLabel = "debt"
	// DebtColor is the label's colour; the first debt issue creates the label.
	DebtColor = "C5DEF5"
	// DebtDescription is the label's description.
	DebtDescription = "Work found out of a change's scope, filed with bin/harness debt"
	// MaxDebtPerBranch is how many debt issues one branch files: past it, the work is the branch's.
	MaxDebtPerBranch = 3
)

// Debt is work found out of a change's scope: a file or a directory to split, a cleanup, a fix.
type Debt struct {
	Title string
	// Path is the file or directory the work is about.
	Path string
	// Why says what needs doing, and why it was not done in the change.
	Why string
	// Branch and Head are the change it was found in.
	Branch, Head string
}

// pathLine and branchLine are the body's lines a duplicate and the cap are found by.
const (
	pathLine   = "**Path:** `%s`"
	branchLine = "**Found on:** `%s`"
)

// IssueTitle is the issue's title.
func (d Debt) IssueTitle() string { return "debt: " + d.Title }

// Body is the issue's body: what, where, why, and the change it was found in.
func (d Debt) Body() string {
	var b strings.Builder

	fmt.Fprintf(&b, pathLine+"\n\n", d.Path)
	fmt.Fprintf(&b, "**Why:** %s\n\n", strings.TrimSpace(d.Why))
	fmt.Fprintf(&b, branchLine+" at `%s`.\n\n", d.Branch, short(d.Head))
	b.WriteString("Filed with `bin/harness debt`: the change that found it left it out of its scope rather than squeeze it in. " +
		"`make next` lists open debt. Close this issue from the pull request that does the work (\"Closes #N\").\n")

	return b.String()
}

// Filed is the open debt issue about the same path, if there is one: a path is filed once.
func Filed(open []triage.Issue, path string) (triage.Issue, bool) {
	line := fmt.Sprintf(pathLine, path)

	for _, i := range open {
		if strings.Contains(i.Body, line) {
			return i, true
		}
	}

	return triage.Issue{}, false
}

// FiledFrom counts the open debt issues a branch filed.
func FiledFrom(open []triage.Issue, branch string) int {
	line := fmt.Sprintf(branchLine, branch)
	n := 0

	for _, i := range open {
		if strings.Contains(i.Body, line) {
			n++
		}
	}

	return n
}

func short(sha string) string {
	const n = 7
	if len(sha) > n {
		return sha[:n]
	}

	return sha
}

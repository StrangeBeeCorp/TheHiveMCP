package evidence

import (
	"fmt"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
)

// Coverage is how a review of a branch covers it at a later commit, rebased or not: its commits as
// the review read them (Replayed), replayed with a resolved conflict (Rebased), or added since and
// never reviewed (Added). A rebase asks for no new review: these lists are for whoever merges.
type Coverage struct {
	// Review is the commit the review judged.
	Review   string        `json:"review"`
	Replayed int           `json:"replayed"`
	Rebased  []gitx.Commit `json:"rebased,omitempty"`
	Added    []gitx.Commit `json:"added,omitempty"`
}

// Covers reports whether the review read any of the branch's commits: one it read none of is not
// covered.
func (c Coverage) Covers() bool { return c.Replayed+len(c.Rebased) > 0 }

// Closer reports whether c covers the branch more closely than o: fewer commits added since the
// review, then fewer changed by a rebase. A review of a pull request lower in a stack reads some of
// the branch's commits, but leaves this branch's own unread.
func (c Coverage) Closer(o Coverage) bool {
	if len(c.Added) != len(o.Added) {
		return len(c.Added) < len(o.Added)
	}

	return len(c.Rebased) < len(o.Rebased)
}

// Cover pairs the commits a review read (judged) with the branch's commits now (current). A
// commit with the same patch is replayed as the review read it; then one with the same subject,
// author and author date, which a rebase keeps, is the same commit changed by the rebase; any
// other is added since. Each judged commit pairs with one commit at most.
func Cover(judged, current []gitx.Commit) Coverage {
	used := make([]bool, len(judged))

	// pair marks the first unused judged commit that same accepts, and says whether there was one.
	pair := func(same func(gitx.Commit) bool) bool {
		for i, j := range judged {
			if !used[i] && same(j) {
				used[i] = true

				return true
			}
		}

		return false
	}

	var c Coverage

	unpaired := make([]gitx.Commit, 0, len(current))

	for _, k := range current {
		if k.PatchID != "" && pair(func(j gitx.Commit) bool { return j.PatchID == k.PatchID }) {
			c.Replayed++

			continue
		}

		unpaired = append(unpaired, k)
	}

	for _, k := range unpaired {
		if pair(func(j gitx.Commit) bool { return identity(j) == identity(k) }) {
			c.Rebased = append(c.Rebased, k)

			continue
		}

		c.Added = append(c.Added, k)
	}

	return c
}

// identity is what a rebase keeps of a commit, whatever its patch becomes.
func identity(c gitx.Commit) string { return c.Subject + "\x00" + c.Author }

// coverageNote is the Gate evidence's paragraph on a review that read the branch at an earlier
// commit, with the commits it did not read as they are now.
func coverageNote(c Coverage) string {
	var b strings.Builder

	total := c.Replayed + len(c.Rebased) + len(c.Added)
	fmt.Fprintf(&b, "\n**Review:** the review of `%s` read this branch at an earlier commit. Of its %d commits: %d as it read them, "+
		"%d changed by a rebase, %d added since and not reviewed.", short(c.Review), total, c.Replayed, len(c.Rebased), len(c.Added))

	if len(c.Rebased)+len(c.Added) > 0 {
		b.WriteString(" Read the ones below closely.")
	}

	b.WriteString("\n")

	for _, k := range c.Rebased {
		fmt.Fprintf(&b, "- changed by the rebase: `%s` %s\n", short(k.SHA), k.Subject)
	}

	for _, k := range c.Added {
		fmt.Fprintf(&b, "- added since the review: `%s` %s\n", short(k.SHA), k.Subject)
	}

	return b.String()
}

package evidence

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
)

const (
	jean    = "jean@example.com 2026-10-06T10:00:00+02:00"
	jeanPM  = "jean@example.com 2026-10-06T15:00:00+02:00"
	someone = "someone@example.com 2026-10-06T10:00:00+02:00"

	roundOne = "fix: round 1"
	fixLint  = "fix: lint"

	reviewSHA = "9876543fed"
)

// The review read three commits. After a rebase, one is replayed as it was, one with a resolved
// conflict (the same commit, its author and date kept, another patch), one was dropped, and a
// fourth was added since.
func TestAReviewCoversTheCommitsItReadAfterARebase(t *testing.T) {
	t.Parallel()

	judged := []gitx.Commit{
		{SHA: "a1", Subject: "feat: the tool", Author: jean, PatchID: "p1"},
		{SHA: "a2", Subject: roundOne, Author: jean, PatchID: "p2"},
		{SHA: "a3", Subject: "docs: upstream already has it", Author: jean, PatchID: "p3"},
	}
	current := []gitx.Commit{
		{SHA: "b1", Subject: "feat: the tool", Author: jean, PatchID: "p1"},
		{SHA: "b2", Subject: roundOne, Author: jean, PatchID: "p2-resolved"},
		{SHA: "b4", Subject: "chore: the restack's fallout", Author: jeanPM, PatchID: "p4"},
	}

	c := Cover(judged, current)
	assert.True(t, c.Covers())
	assert.Equal(t, 1, c.Replayed)
	assert.Equal(t, []gitx.Commit{current[1]}, c.Rebased, "changed by the rebase")
	assert.Equal(t, []gitx.Commit{current[2]}, c.Added, "added since the review, not reviewed")
}

// A commit is paired once, by its patch first; the same subject is the same commit only with the
// same author and author date, which a rebase keeps: another branch's "fix: lint" is another
// commit.
func TestEachReviewedCommitPairsWithOneCommit(t *testing.T) {
	t.Parallel()

	judged := []gitx.Commit{{Subject: fixLint, Author: jean, PatchID: "p1"}}
	current := []gitx.Commit{
		{SHA: "b1", Subject: fixLint, Author: jean, PatchID: "p9"},
		{SHA: "b2", Subject: fixLint, Author: jean, PatchID: "p1"},
	}

	c := Cover(judged, current)
	assert.Equal(t, 1, c.Replayed, "the same patch wins over the same subject")
	assert.Empty(t, c.Rebased)
	assert.Equal(t, []gitx.Commit{current[0]}, c.Added)

	for name, other := range map[string]string{"another author": someone, "another date": jeanPM} {
		c = Cover(judged, []gitx.Commit{{SHA: "b3", Subject: fixLint, Author: other, PatchID: "p9"}})
		assert.False(t, c.Covers(), name)
	}
}

// Of two reviews, the one that leaves fewer commits unread covers the branch: a stacked pull
// request's own review, not the review of a pull request below it, which read more of the stack's
// commits but none of this branch's.
func TestTheClosestReviewLeavesTheFewestCommitsUnread(t *testing.T) {
	t.Parallel()

	below := Coverage{Replayed: 27, Added: make([]gitx.Commit, 63)}
	own := Coverage{Replayed: 2, Added: make([]gitx.Commit, 1)}
	assert.True(t, own.Closer(below))
	assert.False(t, below.Closer(own))

	rebased := Coverage{Replayed: 1, Rebased: make([]gitx.Commit, 1), Added: make([]gitx.Commit, 1)}
	assert.True(t, own.Closer(rebased), "as many added: fewer changed by a rebase")
	assert.False(t, own.Closer(own), "no closer than itself")
}

// A branch none of whose commits the review read is not covered: its commits are all added.
func TestAReviewOfOtherCommitsCoversNothing(t *testing.T) {
	t.Parallel()

	c := Cover([]gitx.Commit{{Subject: "feat: another branch", Author: jean, PatchID: "p1"}},
		[]gitx.Commit{{SHA: "b1", Subject: "feat: this one", Author: jean, PatchID: "p2"}})
	assert.False(t, c.Covers())
	assert.False(t, Cover(nil, nil).Covers(), "no commit, no coverage")
}

// The Gate evidence lists the commits the review did not read as they are now, for whoever merges.
func TestTheGateEvidenceListsWhatTheReviewDidNotRead(t *testing.T) {
	t.Parallel()

	ev := Evidence{Head: "1234567abc", Base: "abcdef0123", Branch: "feat/x", Coverage: &Coverage{
		Review: reviewSHA, Replayed: 4,
		Rebased: []gitx.Commit{{SHA: "aaaaaaa111", Subject: roundOne}},
		Added:   []gitx.Commit{{SHA: "bbbbbbb222", Subject: "chore: the restack's fallout"}},
	}}

	md := Markdown(ev, nil)
	assert.Contains(t, md, "**Review:** the review of `9876543` read this branch before a rebase. Of its 6 commits, 4 are as it read them, "+
		"1 was changed by the rebase, and 1 was added since and is not reviewed: read those two closely.")
	assert.Contains(t, md, "- changed by the rebase: `aaaaaaa` fix: round 1")
	assert.Contains(t, md, "- added since the review: `bbbbbbb` chore: the restack's fallout")

	ev.Coverage = &Coverage{Review: reviewSHA, Replayed: 5}
	assert.Contains(t, Markdown(ev, nil), "**Review:** the review of `9876543` read this branch before a rebase. Of its 5 commits, 5 are as it read them.")

	ev.Coverage = &Coverage{Review: reviewSHA, Replayed: 1, Added: []gitx.Commit{{SHA: "bbbbbbb222", Subject: "docs: link"}}}
	assert.Contains(t, Markdown(ev, nil), "Of its 2 commits, 1 is as it read them, and 1 was added since")

	ev.Coverage = nil
	assert.NotContains(t, Markdown(ev, nil), "**Review:**", "a review of HEAD itself needs no note")
}

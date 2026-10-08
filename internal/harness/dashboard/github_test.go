package dashboard

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeGH answers gh's calls by their first words, and fails the ones it has no answer for.
type fakeGH map[string]string

var errNoAnswer = errors.New("HTTP 404: not found")

func (f fakeGH) Run(_ context.Context, args ...string) ([]byte, error) {
	for prefix, answer := range f {
		if strings.HasPrefix(strings.Join(args, " "), prefix) {
			return []byte(answer), nil
		}
	}

	return nil, errNoAnswer
}

const prLines = `{"number":2,"title":"b","body":"![review](https://img.shields.io/badge/review-9%2F10-brightgreen) **9/10** · review medium · 1 round · 3 fixed\n45,298 of 400,000 tokens","baseRefName":"main","createdAt":"2026-10-02T10:00:00Z","mergedAt":"2026-10-04T10:00:00Z","commits":{"nodes":[{"commit":{"authoredDate":"2026-10-01T10:00:00Z"}}]}}
{"number":1,"title":"a","body":"","baseRefName":"feat/x","createdAt":"2026-10-01T10:00:00Z","mergedAt":"2026-10-01T12:00:00Z","commits":{"nodes":[]}}
`

// GitHub's parts are read at once: the pull requests with their first commit and what their
// descriptions say of the review, the runs, the issues. A part that fails is named alone.
func TestGitHubIsReadPartByPart(t *testing.T) {
	t.Parallel()

	gh := fakeGH{
		"api graphql":                prLines,
		"run list --workflow ci.yml": `[{"createdAt":"2026-10-01T10:00:00Z","updatedAt":"2026-10-01T10:12:00Z","conclusion":"success","attempt":2,"headBranch":"main","event":"push"}]`,
		"issue list --label nightly": `[{"number":5,"title":"a finding","createdAt":"2026-09-01T00:00:00Z"}]`,
		"issue list --label debt":    `[]`,
	}

	d, errs := readGitHub(t.Context(), gh)

	require.Len(t, d.PRs, 2)
	assert.Equal(t, PR{
		Number: 2, Title: "b", Base: mainBranch, Created: at("2026-10-02T10:00:00Z"), Merged: at("2026-10-04T10:00:00Z"),
		FirstCommit: at("2026-10-01T10:00:00Z"), Grade: 9, ReviewTokens: 45298, Fixed: 3,
	}, d.PRs[0])
	assert.Equal(t, d.PRs[1].Created, d.PRs[1].FirstCommit, "no commit read: the pull request's creation")
	require.Len(t, d.CI, 1)
	assert.Equal(t, 2, d.CI[0].Attempt)
	assert.Len(t, d.Findings, 1)

	require.Len(t, errs, 1, "only the nightly runs failed")
	require.ErrorIs(t, errs[SourceNightly], errNoAnswer)

	_, errs = readGitHub(t.Context(), nil)
	require.ErrorIs(t, errs[SourceGitHub], errNoGH)

	_, errs = readGitHub(t.Context(), fakeGH{"api graphql": "{not json"})
	require.Error(t, errs[SourcePRs])
}

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}

	return t
}

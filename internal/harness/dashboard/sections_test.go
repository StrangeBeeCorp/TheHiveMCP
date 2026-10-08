package dashboard

import (
	"html"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/evidence"
)

// The fixtures' tag, gate and conclusion.
const (
	tagNew  = "v1.1.0"
	gateIT  = "integration"
	failure = "failure"
)

// fixture is a month of data with every source read but the notes and the last local run.
func fixture() Data {
	now := at("2026-10-08T12:00:00Z")
	day := func(n int) time.Time { return now.AddDate(0, 0, -n) }

	return Data{
		Since: now.AddDate(0, 0, -28), Now: now,
		Tags: []Tag{{Name: tagNew, When: day(2)}, {Name: "v1.0.0", When: day(12)}},
		Notes: []NoteRun{
			{Note: evidence.Note{Started: day(1), Seconds: 600, Gates: []evidence.GateTime{{Gate: "lint", Seconds: 10}, {Gate: gateIT, Seconds: 300}, {Gate: "mutation", Seconds: 200}, {Gate: "fmt", Seconds: 3}}}, MachineLabel: "m"},
			{Note: evidence.Note{Started: day(2), Seconds: 300, Gates: []evidence.GateTime{{Gate: "lint", Seconds: 12}, {Gate: gateIT, Seconds: 400}, {Gate: "mutation", Seconds: 100}, {Gate: "fmt", Seconds: 3}}}, MachineLabel: "m"},
		},
		GitHub: GitHubData{
			PRs: []PR{
				{Number: 1, Base: mainBranch, Merged: day(1), FirstCommit: day(3), Grade: 9, ReviewTokens: 50000, Fixed: 2},
				{Number: 2, Base: mainBranch, Merged: day(5), FirstCommit: day(6), Grade: 7, ReviewTokens: 150000, Fixed: 1},
				{Number: 3, Base: mainBranch, Merged: day(6), FirstCommit: day(7)},
			},
			Nightly: []Run{{Created: day(1), Conclusion: success}, {Created: day(2), Conclusion: failure}, {Created: day(3), Conclusion: success}, {Created: day(4), Conclusion: success}},
		},
		Tickets: []Ticket{{Key: "X-1", Summary: "one", Started: day(10), Done: day(4)}, {Key: "X-2", Summary: "two", Started: day(3), Done: day(1)}},
		Tree:    TreeData{NearLimit: 2, Nolint: 5, TODO: 1},
		Local: LocalRun{Coverage: 70, Mutation: []Mutation{
			{Package: "far", Efficacy: 95, Floor: 90}, {Package: "near", Efficacy: 90.5, Floor: 90}, {Package: "under", Efficacy: 89, Floor: 90},
		}},
		Missing: map[string]string{SourceNotes: "origin's notes could not be fetched", SourceLocal: "no make pr run"},
	}
}

// tiles reads a block's tiles by label.
func tiles(b Block) map[string]string {
	out := map[string]string{}
	for _, t := range b.Tiles {
		out[t.Label] = t.Value
	}

	return out
}

func block(t *testing.T, blocks []Block, title string) Block {
	t.Helper()

	for _, b := range blocks {
		if b.Title == title {
			return b
		}
	}

	require.Failf(t, "no block", "no block %q", title)

	return Block{}
}

// The quality section's numbers: the mean grade of the graded pull requests, the coverage, the
// tree's counts, and the mutation scores closest to their floor first.
func TestTheQualitySectionsNumbers(t *testing.T) {
	t.Parallel()

	d := fixture()
	blocks := quality(d, mergedPRs(d))

	q := tiles(blocks[0])
	assert.Equal(t, "8.0/10", q["Mean grade"], "9 and 7; the ungraded one is left out")
	assert.Equal(t, "70.0 %", q["Coverage"])
	assert.Equal(t, "2", q["Files near their limit"])
	assert.Equal(t, "5", q["//nolint"])

	mutation := block(t, blocks, "Mutation scores, last make pr")
	assert.Equal(t, []string{"under", "near", "far"}, []string{mutation.Rows[0][0], mutation.Rows[1][0], mutation.Rows[2][0]})
	assert.Equal(t, "-1.0", mutation.Rows[0][3])

	assert.Equal(t, "–", tiles(quality(Data{Local: LocalRun{Coverage: -1}}, nil)[0])["Mean grade"], "no graded pull request: no NaN")
}

// The delivery section: release frequency, the nightly pass rate, and the review's cost (median
// tokens) and yield (findings fixed per 100k tokens).
func TestTheDeliverySectionsNumbers(t *testing.T) {
	t.Parallel()

	d := fixture()
	blocks := delivery(d, mainRuns(d), mergedPRs(d))

	dora, rest := tiles(blocks[0]), tiles(blocks[1])
	assert.Equal(t, "2.1", dora["Deployment frequency"], "2 tags in 28 days")
	assert.Equal(t, "1.0 d", dora["Lead time for changes"], "median of 2, 1 and 1 days")
	assert.Equal(t, "75 %", rest["Nightly pass rate"])
	assert.Equal(t, "100k", rest["Review cost"], "median of 50k and 150k")
	assert.Equal(t, "1.5", rest["Review yield"], "3 fixed for 200k tokens")

	assert.Equal(t, "–", tiles(delivery(Data{}, nil, nil)[1])["Review yield"], "no review: no NaN")
}

// Test time names each machine's slowest gates by their median; the releases table gives the gap
// between tags; the tickets' lead time is their median, and the last ones done come first.
func TestTestTimeReleasesAndTickets(t *testing.T) {
	t.Parallel()

	d := fixture()

	machines := block(t, testTime(d, nil), "make pr by machine")
	assert.Equal(t, []string{"m", "2", "7.5 min", "integration 350 s, mutation 150 s, lint 11 s"}, machines.Rows[0])

	releases := block(t, flow(d), labelReleases)
	assert.Equal(t, []string{tagNew, "6 Oct 2026", "10.0 d"}, releases.Rows[0])
	assert.Empty(t, releases.Rows[1][2], "the first tag has none before it")

	lead := leadTime(d, mergedPRs(d))
	tickets := block(t, lead, "Last tickets done")
	assert.Equal(t, []string{"X-2", "two", "2.0 d"}, tickets.Rows[0])
	assert.Equal(t, "6.0 d", tickets.Rows[1][2])
}

// A source that could not be read is said in each section it feeds, on the page itself.
func TestTheSectionsSayWhichSourceTheyCouldNotRead(t *testing.T) {
	t.Parallel()

	var out strings.Builder
	require.NoError(t, Render(&out, Build(fixture())))

	page := out.String()
	test := page[strings.Index(page, `id="test-time"`):strings.Index(page, `id="layers"`)]
	assert.Contains(t, test, html.EscapeString(SourceNotes)+" could not be read:</b> origin&#39;s notes could not be fetched")

	quality := page[strings.Index(page, `id="quality"`):strings.Index(page, `id="delivery"`)]
	assert.Contains(t, quality, html.EscapeString(SourceLocal)+" could not be read:</b> no make pr run")
	assert.NotContains(t, quality, html.EscapeString(SourceNotes), "a section names only its own sources")
}

package dashboard

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"
	_ "time/tzdata" // Europe/Paris, whatever the image holds

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/evidence"
)

// A breakdown shows the largest first and folds the rest into Other; a person is one person
// whatever the capitals of their name.
func TestABreakdownFoldsTheTailAndKnowsOnePerson(t *testing.T) {
	t.Parallel()

	var keys []string

	for i := range 10 {
		for range 10 - i {
			keys = append(keys, fmt.Sprintf("k%d", i))
		}
	}

	items := counted(keys)
	require.Len(t, items, topItems+1)
	assert.Equal(t, "k0", items[0].Label)
	assert.Equal(t, Item{Label: other, Value: 6, Text: "6", Tip: "Other: 6 of 55 (11 %)"}, items[topItems], "k7, k8 and k9: 3+2+1")

	const ann = "Ann Lee"

	assert.Equal(t, []string{ann, ann, ann, "Bo"}, samePerson([]string{ann, "ANN LEE", ann, "Bo"}))
}

// Main's runs give the failure rate, the time from a red run to the next green one, and the runs
// green only on a retry.
func TestMainsRunsGiveFailureRestoreAndFlakes(t *testing.T) {
	t.Parallel()

	base := at("2026-10-01T10:00:00Z")
	runs := []Run{
		{Created: base, Updated: base.Add(10 * time.Minute), Conclusion: success, Attempt: 1},
		{Created: base.Add(time.Hour), Updated: base.Add(70 * time.Minute), Conclusion: failure, Attempt: 1},
		{Created: base.Add(2 * time.Hour), Updated: base.Add(130 * time.Minute), Conclusion: failure, Attempt: 1},
		{Created: base.Add(3 * time.Hour), Updated: base.Add(4 * time.Hour), Conclusion: success, Attempt: 2},
	}

	assert.InDelta(t, 50, failureRate(runs), 0.01)
	assert.InDelta(t, 3, timeToRestore(runs), 0.01, "red at 11:00, green again at 14:00")
	assert.Equal(t, 1, flakyRuns(runs))
	assert.True(t, math.IsNaN(failureRate(nil)))
	assert.True(t, math.IsNaN(median(nil)))
	assert.InDelta(t, 2.5, median([]float64{4, 1, 2, 3}), 0.01)
}

// The weeks run Monday to Sunday from the window's start, a week without entries at 0.
func TestASeriesHasEveryWeekOfTheWindow(t *testing.T) {
	t.Parallel()

	since, now := at("2026-09-30T12:00:00Z"), at("2026-10-14T12:00:00Z") // a Wednesday, two weeks on
	times := []time.Time{at("2026-09-29T08:00:00Z"), at("2026-10-01T08:00:00Z"), at("2026-10-13T08:00:00Z")}

	items := weekly(since, now, times, func(t time.Time) time.Time { return t }, func(in []time.Time) float64 { return float64(len(in)) }, count)
	require.Len(t, items, 3)
	assert.Equal(t, []string{"Sep 28", "Oct 5", "Oct 12"}, []string{items[0].Label, items[1].Label, items[2].Label})
	assert.Equal(t, []float64{2, 0, 1}, []float64{items[0].Value, items[1].Value, items[2].Value})
}

// Across a change of time (Paris goes back an hour on 25 October 2026) the weeks still start on
// Monday at midnight, and an entry late on Sunday stays in its week.
func TestAWeekAcrossAChangeOfTimeStaysMondayToMonday(t *testing.T) {
	t.Parallel()

	paris, err := time.LoadLocation("Europe/Paris")
	require.NoError(t, err)

	since := time.Date(2026, 10, 19, 12, 0, 0, 0, paris)
	sunday := time.Date(2026, 11, 1, 23, 30, 0, 0, paris)

	items := weekly(since, since.AddDate(0, 0, 14), []time.Time{sunday}, func(t time.Time) time.Time { return t },
		func(in []time.Time) float64 { return float64(len(in)) }, count)
	require.Len(t, items, 3)
	assert.Equal(t, []string{"Oct 19", "Oct 26", "Nov 2"}, []string{items[0].Label, items[1].Label, items[2].Label})
	assert.Equal(t, []float64{0, 1, 0}, []float64{items[0].Value, items[1].Value, items[2].Value}, "Sunday 23:30 is in the week of Oct 26")
}

// The page is drawn from the data: its headline numbers, its sections, the sources it could not
// read in the sections they feed, and every text escaped.
func TestThePageIsDrawnFromTheDataAndSaysWhatItCouldNotRead(t *testing.T) {
	t.Parallel()

	now := at("2026-10-08T12:00:00Z")
	d := Data{
		Repo: "Owner/Repo", Head: "abc1234", Since: now.AddDate(0, 0, -28), Now: now, Built: 2 * time.Second,
		Commits: []Commit{
			{Author: "Ann", When: now, Model: "Claude Opus 5.5", Effort: "high", Skills: []string{"develop-feature"}},
			{Author: "Bo <script>", When: now},
		},
		Merges: []time.Time{now.AddDate(0, 0, -1)},
		Tags:   []Tag{{Name: tagNew, When: now.AddDate(0, 0, -2)}, {Name: "v1.0.0", When: now.AddDate(0, 0, -12)}},
		Notes: []NoteRun{{Note: evidence.Note{Started: now, Seconds: 600, Gates: []evidence.GateTime{{Gate: gateIT, Seconds: 345}}},
			MachineLabel: "darwin/arm64, 15 CPUs"}},
		GitHub: GitHubData{
			PRs:  []PR{{Number: 1, Base: mainBranch, Merged: now.AddDate(0, 0, -1), FirstCommit: now.AddDate(0, 0, -3), Grade: 9, ReviewTokens: 50000, Fixed: 2}},
			CI:   []Run{{Created: now.AddDate(0, 0, -1), Updated: now.AddDate(0, 0, -1).Add(10 * time.Minute), Conclusion: success, Branch: mainBranch, Event: "push", Attempt: 1}},
			Debt: []Issue{{Number: 7, Title: "debt: a flaky test", Created: now.AddDate(0, 0, -4)}},
		},
		Tree:    TreeData{Layers: Layers{Unit: 5, Integration: 2}},
		Local:   LocalRun{Coverage: 64.8, Head: "abc1234", Started: now, Verdict: "PASS"},
		Missing: map[string]string{SourceJira: "not configured", SourceGitHub: "gh is not installed"},
	}

	p := Build(d)

	tiles := map[string]string{}
	for _, tile := range p.Tiles {
		tiles[tile.Label] = tile.Value
	}

	assert.Equal(t, map[string]string{
		"Merged pull requests": "1", "Commits by agents": "50 %", "PR lead time": "2.0 d",
		"CI on main": "10.0 min", "Main CI failing": "0 %", labelReleases: "2.1",
	}, tiles)

	notices := map[string][]string{}

	for _, s := range p.Sections {
		for _, n := range s.Notices {
			notices[s.ID] = append(notices[s.ID], n.Source)
		}
	}

	assert.Equal(t, []string{SourcePRs, SourceJira}, slices.Sorted(slices.Values(notices["lead-time"])), "gh missing leaves out every GitHub part")
	assert.NotContains(t, notices, "layers", "the tree was read")

	var html strings.Builder
	require.NoError(t, Render(&html, p))

	page := html.String()
	assert.Contains(t, page, "Owner/Repo: repository health")
	assert.Contains(t, page, "Bo &lt;script&gt;", "a name is escaped in the chart")
	assert.Contains(t, page, "darwin/arm64, 15 CPUs")
	assert.Contains(t, page, "debt: a flaky test")
	assert.Contains(t, page, "Not read, so some sections are partial")
}

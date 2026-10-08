package dashboard

import (
	"cmp"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	// topItems is how many items a breakdown shows before folding the rest into "Other".
	topItems = 7
	// recentRuns is how many runs a headline takes its median from.
	recentRuns = 30
	// mainBranch is the branch the page measures.
	mainBranch = "main"
	// success is a run's conclusion when it passed.
	success   = "success"
	daysAWeek = 7
	hundred   = 100.0 // a percent's whole
	// aPerson is the model of a commit no agent wrote.
	aPerson = "none (a person)"
	other   = "Other"
)

// Item is one bar of a breakdown or one week of a series.
type Item struct {
	Label string
	Value float64
	// Text is how the value reads ("12", "3.4 min"); Tip adds what a hover shows.
	Text string
	Tip  string
}

// weekly buckets times into the weeks from since to now, Monday first, applying value to each
// week's entries; a week without one is 0.
func weekly[T any](since, now time.Time, entries []T, when func(T) time.Time, value func([]T) float64, text func(float64) string) []Item {
	start := monday(since)

	var items []Item

	for w := start; !w.After(now); w = w.AddDate(0, 0, daysAWeek) {
		var in []T

		for _, e := range entries {
			if t := when(e); !t.Before(w) && t.Before(w.AddDate(0, 0, daysAWeek)) {
				in = append(in, e)
			}
		}

		v := 0.0
		if len(in) > 0 {
			v = value(in)
		}

		items = append(items, Item{Label: w.Format("Jan 2"), Value: v, Text: text(v), Tip: fmt.Sprintf("week of %s: %s (%d)", w.Format("2 Jan 2006"), text(v), len(in))})
	}

	return items
}

func monday(t time.Time) time.Time {
	t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())

	return t.AddDate(0, 0, -((int(t.Weekday()) + 6) % 7)) //nolint:mnd // Monday is 0, Sunday 6
}

// counted is a breakdown of how often each key appears, the largest first, past topItems folded
// into Other.
func counted(keys []string) []Item {
	n := map[string]int{}
	for _, k := range keys {
		n[k]++
	}

	labels := slices.Collect(maps.Keys(n))
	slices.SortFunc(labels, func(a, b string) int { return cmp.Or(n[b]-n[a], strings.Compare(a, b)) })

	var items []Item

	rest := 0

	for i, l := range labels {
		if i >= topItems {
			rest += n[l]

			continue
		}

		items = append(items, Item{Label: l, Value: float64(n[l]), Text: strconv.Itoa(n[l])})
	}

	if rest > 0 {
		items = append(items, Item{Label: other, Value: float64(rest), Text: strconv.Itoa(rest)})
	}

	total := len(keys)
	for i := range items {
		items[i].Tip = fmt.Sprintf("%s: %s of %d (%.0f %%)", items[i].Label, items[i].Text, total, hundred*items[i].Value/float64(total))
	}

	return items
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return math.NaN()
	}

	s := slices.Sorted(slices.Values(v))
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}

	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

func count(n float64) string { return fmt.Sprintf("%.0f", n) }

func minutes(m float64) string {
	if math.IsNaN(m) {
		return "–"
	}

	return fmt.Sprintf("%.1f min", m)
}

func days(d float64) string {
	if math.IsNaN(d) {
		return "–"
	}

	return fmt.Sprintf("%.1f d", d)
}

func percent(p float64) string {
	if math.IsNaN(p) {
		return "–"
	}

	return fmt.Sprintf("%.0f %%", p)
}

// mainRuns are the CI runs of pushes to main within the window that finished, oldest first.
func mainRuns(d Data) []Run {
	var runs []Run

	for _, r := range d.GitHub.CI {
		if r.Branch == mainBranch && r.Event == "push" && r.Conclusion != "" && !r.Created.Before(d.Since) {
			runs = append(runs, r)
		}
	}

	slices.SortFunc(runs, func(a, b Run) int { return a.Created.Compare(b.Created) })

	return runs
}

func runMinutes(r Run) float64 { return r.Updated.Sub(r.Created).Minutes() }

// mergedPRs are the pull requests merged into main within the window.
func mergedPRs(d Data) []PR {
	var prs []PR

	for _, p := range d.GitHub.PRs {
		if p.Base == mainBranch && !p.Merged.Before(d.Since) {
			prs = append(prs, p)
		}
	}

	return prs
}

func leadDays(p PR) float64 { return p.Merged.Sub(p.FirstCommit).Hours() / 24 } //nolint:mnd // hours in a day

// failureRate is the share of runs that did not succeed, in percent.
func failureRate(runs []Run) float64 {
	if len(runs) == 0 {
		return math.NaN()
	}

	failed := 0

	for _, r := range runs {
		if r.Conclusion != success {
			failed++
		}
	}

	return hundred * float64(failed) / float64(len(runs))
}

// timeToRestore is the median time, in hours, from a failed run to the next one that succeeded.
func timeToRestore(runs []Run) float64 {
	var waits []float64

	var broke time.Time

	for _, r := range runs {
		switch {
		case r.Conclusion != success && broke.IsZero():
			broke = r.Created
		case r.Conclusion == success && !broke.IsZero():
			waits = append(waits, r.Updated.Sub(broke).Hours())
			broke = time.Time{}
		}
	}

	return median(waits)
}

// flakyRuns are the runs that succeeded only on a retry.
func flakyRuns(runs []Run) int {
	n := 0

	for _, r := range runs {
		if r.Conclusion == success && r.Attempt > 1 {
			n++
		}
	}

	return n
}

// ages are how many days each issue has been open.
func ages(now time.Time, issues []Issue) []float64 {
	out := make([]float64, 0, len(issues))
	for _, i := range issues {
		out = append(out, now.Sub(i.Created).Hours()/24) //nolint:mnd // hours in a day
	}

	return out
}

// modelOf names a commit's model for the breakdown: the agent's, or a person.
func modelOf(c Commit) string {
	if c.Model == "" {
		return aPerson
	}

	return c.Model
}

// releasesPerMonth is how many v* tags the window holds, per 30 days.
func releasesPerMonth(d Data) float64 {
	n := 0

	for _, t := range d.Tags {
		if !t.When.Before(d.Since) {
			n++
		}
	}

	months := d.Now.Sub(d.Since).Hours() / 24 / 30 //nolint:mnd // a month of 30 days
	if months <= 0 {
		return math.NaN()
	}

	return float64(n) / months
}

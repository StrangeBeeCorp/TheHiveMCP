package dashboard

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Page is what the template draws.
type Page struct {
	Repo, Head, Window, Built string
	Tiles                     []Tile
	Sections                  []Section
	// Missing lists every source the page could not read, and why.
	Missing []Notice
}

// Tile is a headline number.
type Tile struct {
	Label, Value, Note string
}

// Notice says a source could not be read.
type Notice struct {
	Source, Why string
}

// Section is one part of the page.
type Section struct {
	ID, Title, Lead string
	// Notices are the sources it reads that could not be read.
	Notices []Notice
	Blocks  []Block
}

// Block is one chart, table or row of tiles; Kind picks how it is drawn.
type Block struct {
	Kind  string // "bars" (over time), "hbars" (a breakdown), "table", "tiles"
	Title string
	Note  string
	Items []Item
	Head  []string
	Rows  [][]string
	Tiles []Tile
}

// labelReleases names the release tags, in a tile and a table.
const labelReleases = "Releases"

const (
	kindBars  = "bars"
	kindHBars = "hbars"
	kindTable = "table"
	kindTiles = "tiles"
)

// Build turns the data into the page.
func Build(d Data) Page {
	p := Page{
		Repo: d.Repo, Head: d.Head, Built: fmt.Sprintf("%s, in %.1f s", d.Now.Format("2 Jan 2006 15:04"), d.Built.Seconds()),
		Window: fmt.Sprintf("%s to %s", d.Since.Format("2 Jan 2006"), d.Now.Format("2 Jan 2006")),
	}

	for _, s := range slices.Sorted(maps.Keys(d.Missing)) {
		p.Missing = append(p.Missing, Notice{Source: s, Why: d.Missing[s]})
	}

	runs, prs := mainRuns(d), mergedPRs(d)

	p.Tiles = headline(d, runs, prs)

	p.Sections = []Section{
		section(d, "contributors", "Contributors", "Who wrote the commits of main and this branch: people, and agents by model, effort and skill (their commits' trailers; Co-Authored-By before them).",
			[]string{SourceGit}, contributors(d)...),
		section(d, "test-time", "Test time", "How long the gates take: CI's runs of main, and every developer's make pr (its notes), by machine.",
			[]string{SourceCI, SourceNotes}, testTime(d, runs)...),
		section(d, "layers", "Tests per layer", "Test functions in the tree, by layer: an integration test reaches StartTheHiveContainer.",
			[]string{SourceTree}, layers(d)...),
		section(d, "flow", "Merges and releases", "Commits that reached main (its first-parent history) each week, and the release tags.",
			[]string{SourceGit}, flow(d)...),
		section(d, "lead-time", "Feature lead time", "From a pull request's first commit to its merge, and from a ticket's first move to In Progress to its resolution.",
			[]string{SourcePRs, SourceJira}, leadTime(d, prs)...),
		section(d, "quality", "Quality", "Pull request grades, coverage and mutation scores, files against the structure limits, and what the code waives.",
			[]string{SourcePRs, SourceLocal, SourceTree}, quality(d, prs)...),
		section(d, "delivery", "Delivery", "DORA-style measures of main, the nightly eval, the review's cost and yield, flaky runs and the open findings.",
			[]string{SourceCI, SourceNightly, SourcePRs, SourceIssues, SourceGit}, delivery(d, runs, prs)...),
	}

	return p
}

func section(d Data, id, title, lead string, sources []string, blocks ...Block) Section {
	s := Section{ID: id, Title: title, Lead: lead, Blocks: blocks}

	for _, src := range sources {
		for missing, why := range d.Missing {
			if missing == src || (src != SourceGitHub && strings.HasPrefix(src, SourceGitHub) && missing == SourceGitHub) {
				s.Notices = append(s.Notices, Notice{Source: src, Why: why})
			}
		}
	}

	return s
}

func headline(d Data, runs []Run, prs []PR) []Tile {
	var ci []float64

	for _, r := range runs[max(0, len(runs)-recentRuns):] {
		if r.Conclusion == success {
			ci = append(ci, runMinutes(r))
		}
	}

	var lead []float64
	for _, p := range prs {
		lead = append(lead, leadDays(p))
	}

	agents := 0

	for _, c := range d.Commits {
		if c.Model != "" {
			agents++
		}
	}

	share := math.NaN()
	if len(d.Commits) > 0 {
		share = hundred * float64(agents) / float64(len(d.Commits))
	}

	weeks := d.Now.Sub(d.Since).Hours() / 24 / 7 //nolint:mnd // a week of 7 days

	return []Tile{
		{Label: "Merged pull requests", Value: count(float64(len(prs))), Note: fmt.Sprintf("%.1f a week", float64(len(prs))/weeks)},
		{Label: "Commits by agents", Value: percent(share), Note: fmt.Sprintf("of %d commits", len(d.Commits))},
		{Label: "PR lead time", Value: days(median(lead)), Note: "median, first commit to merge"},
		{Label: "CI on main", Value: minutes(median(ci)), Note: fmt.Sprintf("median of the last %d green runs", len(ci))},
		{Label: "Main CI failing", Value: percent(failureRate(runs)), Note: fmt.Sprintf("of %d runs", len(runs))},
		{Label: labelReleases, Value: fmt.Sprintf("%.1f", releasesPerMonth(d)), Note: "a month"},
	}
}

func contributors(d Data) []Block {
	var people, models, efforts, skills []string

	trailered := 0

	for _, c := range d.Commits {
		people = append(people, c.Author)
		models = append(models, modelOf(c))

		if c.Effort != "" {
			trailered++

			efforts = append(efforts, c.Effort)
			skills = append(skills, c.Skills...)
		}
	}

	note := fmt.Sprintf("the %d commits with an agent's trailers (from DL-8155 on)", trailered)

	return []Block{
		{Kind: kindHBars, Title: "Commits by person", Note: "a name written with other capitals is the same person", Items: counted(samePerson(people))},
		{Kind: kindHBars, Title: "Commits by model", Items: counted(models)},
		{Kind: kindHBars, Title: "Agents' commits by effort", Note: note, Items: counted(efforts)},
		{Kind: kindHBars, Title: "Skills the agents used", Note: note + "; the repository's own skills", Items: counted(skills)},
	}
}

// samePerson writes each name the way its person writes it most often, whatever its capitals.
func samePerson(names []string) []string {
	spellings := map[string]map[string]int{}

	for _, n := range names {
		k := strings.ToLower(n)
		if spellings[k] == nil {
			spellings[k] = map[string]int{}
		}

		spellings[k][n]++
	}

	out := make([]string, 0, len(names))

	for _, n := range names {
		s := spellings[strings.ToLower(n)]
		out = append(out, slices.MaxFunc(slices.Sorted(maps.Keys(s)), func(a, b string) int { return s[a] - s[b] }))
	}

	return out
}

func testTime(d Data, runs []Run) []Block {
	green := slices.DeleteFunc(slices.Clone(runs), func(r Run) bool { return r.Conclusion != success })

	blocks := []Block{{
		Kind: kindBars, Title: "CI on main, median minutes a week", Note: "green runs of pushes to main, created to last update",
		Items: weekly(d.Since, d.Now, green, func(r Run) time.Time { return r.Created },
			func(in []Run) float64 { return median(each(in, runMinutes)) }, minutes),
	}}

	byMachine := map[string][]NoteRun{}
	for _, n := range d.Notes {
		byMachine[n.MachineLabel] = append(byMachine[n.MachineLabel], n)
	}

	rows := [][]string{}

	for _, m := range slices.Sorted(maps.Keys(byMachine)) {
		ns := byMachine[m]
		total := each(ns, func(n NoteRun) float64 { return n.Seconds / 60 }) //nolint:mnd // seconds in a minute
		rows = append(rows, []string{m, strconv.Itoa(len(ns)), minutes(median(total)), slowest(ns)})
	}

	blocks = append(blocks,
		Block{
			Kind: kindBars, Title: "Developers' make pr, median minutes a week", Note: "every machine's runs, from their notes",
			Items: weekly(d.Since, d.Now, d.Notes, func(n NoteRun) time.Time { return n.Started },
				func(in []NoteRun) float64 { return median(each(in, func(n NoteRun) float64 { return n.Seconds / 60 })) }, minutes), //nolint:mnd // seconds in a minute
		},
		Block{Kind: kindTable, Title: "make pr by machine", Head: []string{"Machine", "Runs", "Median", "Slowest gates (median)"}, Rows: rows},
	)

	return blocks
}

// slowest names the three gates with the longest median time over the runs.
func slowest(ns []NoteRun) string {
	times := map[string][]float64{}

	for _, n := range ns {
		for _, g := range n.Gates {
			times[g.Gate] = append(times[g.Gate], g.Seconds)
		}
	}

	type gate struct {
		name string
		secs float64
	}

	var gates []gate
	for name, s := range times {
		gates = append(gates, gate{name, median(s)})
	}

	slices.SortFunc(gates, func(a, b gate) int { return int(b.secs - a.secs) })

	var out []string
	for _, g := range gates[:min(3, len(gates))] { //nolint:mnd // the three slowest
		out = append(out, fmt.Sprintf("%s %.0f s", g.name, g.secs))
	}

	return strings.Join(out, ", ")
}

func each[T any](in []T, f func(T) float64) []float64 {
	out := make([]float64, 0, len(in))
	for _, e := range in {
		out = append(out, f(e))
	}

	return out
}

func layers(d Data) []Block {
	l := d.Tree.Layers

	return []Block{{Kind: kindHBars, Title: "Test functions", Items: []Item{
		{Label: "unit", Value: float64(l.Unit), Text: strconv.Itoa(l.Unit), Tip: "Test functions that run under make test"},
		{Label: "integration", Value: float64(l.Integration), Text: strconv.Itoa(l.Integration), Tip: "Test functions that start TheHive"},
		{Label: "fuzz", Value: float64(l.Fuzz), Text: strconv.Itoa(l.Fuzz), Tip: "Fuzz functions"},
		{Label: "benchmark", Value: float64(l.Benchmark), Text: strconv.Itoa(l.Benchmark), Tip: "Benchmark functions, held to gates/bench.yaml"},
		{Label: "canary", Value: float64(l.Canaries), Text: strconv.Itoa(l.Canaries), Tip: "gates/canaries: a patch a gate must catch"},
	}}}
}

func flow(d Data) []Block {
	rows := [][]string{}

	tags := slices.SortedFunc(slices.Values(d.Tags), func(a, b Tag) int { return b.When.Compare(a.When) })

	for i, t := range tags {
		gap := ""
		if i+1 < len(tags) {
			gap = days(t.When.Sub(tags[i+1].When).Hours() / 24) //nolint:mnd // hours in a day
		}

		rows = append(rows, []string{t.Name, t.When.Format("2 Jan 2006"), gap})
	}

	return []Block{
		{
			Kind: kindBars, Title: "Commits reaching main a week",
			Items: weekly(d.Since, d.Now, d.Merges, func(t time.Time) time.Time { return t },
				func(in []time.Time) float64 { return float64(len(in)) }, count),
		},
		{Kind: kindTable, Title: labelReleases, Head: []string{"Tag", "Date", "Since the previous"}, Rows: rows},
	}
}

func leadTime(d Data, prs []PR) []Block {
	blocks := []Block{{
		Kind: kindBars, Title: "Pull requests, median days from first commit to merge, by week merged",
		Items: weekly(d.Since, d.Now, prs, func(p PR) time.Time { return p.Merged },
			func(in []PR) float64 { return median(each(in, leadDays)) }, days),
	}}

	if len(d.Tickets) > 0 {
		rows := [][]string{}

		for _, t := range slices.SortedFunc(slices.Values(d.Tickets), func(a, b Ticket) int { return b.Done.Compare(a.Done) })[:min(10, len(d.Tickets))] { //nolint:mnd // the last ten
			rows = append(rows, []string{t.Key, t.Summary, days(t.Done.Sub(t.Started).Hours() / 24)}) //nolint:mnd // hours in a day
		}

		blocks = append(blocks,
			Block{
				Kind: kindBars, Title: "Tickets, median days from In Progress to done, by week done",
				Items: weekly(d.Since, d.Now, d.Tickets, func(t Ticket) time.Time { return t.Done },
					func(in []Ticket) float64 {
						return median(each(in, func(t Ticket) float64 { return t.Done.Sub(t.Started).Hours() / 24 })) //nolint:mnd // hours in a day
					}, days),
			},
			Block{Kind: kindTable, Title: "Last tickets done", Head: []string{"Ticket", "Summary", "Lead time"}, Rows: rows},
		)
	}

	return blocks
}

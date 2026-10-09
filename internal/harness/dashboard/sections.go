package dashboard

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
)

// hundredK is the unit of the review's yield: findings fixed per 100,000 tokens.
const (
	hundredK = 100_000
	thousand = 1_000
)

func quality(d Data, prs []PR) []Block {
	var grades []string

	var sum, graded float64

	for _, p := range prs {
		if p.Grade > 0 {
			grades = append(grades, fmt.Sprintf("%d/10", p.Grade))
			sum += float64(p.Grade)
			graded++
		}
	}

	mean := math.NaN()
	if graded > 0 {
		mean = sum / graded
	}

	cov := "–"
	if d.Local.Coverage >= 0 {
		cov = fmt.Sprintf("%.1f %%", d.Local.Coverage)
	}

	run := "–"
	if !d.Local.Started.IsZero() {
		run = fmt.Sprintf("%s at %.7s, %s", d.Local.Started.Format("2 Jan 15:04"), d.Local.Head, d.Local.Verdict)
	}

	tiles := Block{Kind: kindTiles, Tiles: []Tile{
		{Label: "Mean grade", Value: decimal(mean, "/10"), Note: fmt.Sprintf("of %d graded pull requests", int(graded))},
		{Label: "Coverage", Value: cov, Note: "statements, last make pr: " + run},
		{Label: "Files near their limit", Value: strconv.Itoa(d.Tree.NearLimit), Note: "at 90 % of their line limit or over"},
		{Label: "//nolint", Value: strconv.Itoa(d.Tree.Nolint), Note: "waivers of the linters in Go files"},
		{Label: "TODO", Value: strconv.Itoa(d.Tree.TODO), Note: "in Go files"},
	}}

	mutation := [][]string{}

	for _, m := range slices.SortedFunc(slices.Values(d.Local.Mutation), func(a, b Mutation) int { return int(hundred * (a.Efficacy - a.Floor - b.Efficacy + b.Floor)) }) {
		mutation = append(mutation, []string{m.Package, fmt.Sprintf("%.1f %%", m.Efficacy), fmt.Sprintf("%.0f %%", m.Floor), fmt.Sprintf("%+.1f", m.Efficacy-m.Floor)})
	}

	files := [][]string{}

	for _, f := range d.Tree.Files {
		limit := strconv.Itoa(f.Limit)
		if f.Cap > 0 {
			limit = fmt.Sprintf("%d (capped)", f.Cap)
		}

		files = append(files, []string{f.Path, strconv.Itoa(f.Lines), limit})
	}

	dirs := [][]string{}
	for _, dir := range d.Tree.Dirs {
		dirs = append(dirs, []string{dir.Path, strconv.Itoa(dir.Files), strconv.Itoa(dir.Limit)})
	}

	return []Block{
		tiles,
		{Kind: kindHBars, Title: "Pull request grades", Note: "the /10 badge of each merged description", Items: counted(grades)},
		{Kind: kindTable, Title: "Mutation scores, last make pr", Head: []string{"Package", "Efficacy", "Floor", "Margin"}, Rows: mutation},
		{Kind: kindTable, Title: "Largest code files", Head: []string{"File", "Lines", "Limit"}, Rows: files},
		{Kind: kindTable, Title: "Fullest directories", Head: []string{"Directory", "Code files", "Limit"}, Rows: dirs},
	}
}

func delivery(d Data, runs []Run, prs []PR) []Block {
	lead := each(prs, leadDays)

	var tokens []float64

	var fixed, spent float64

	for _, p := range prs {
		if p.ReviewTokens > 0 {
			tokens = append(tokens, float64(p.ReviewTokens))
			spent += float64(p.ReviewTokens)
			fixed += float64(p.Fixed)
		}
	}

	yield := math.NaN()
	if spent > 0 {
		yield = fixed / spent * hundredK
	}

	var nightly []Run

	for _, r := range d.GitHub.Nightly {
		if r.Conclusion != "" && !r.Created.Before(d.Since) {
			nightly = append(nightly, r)
		}
	}

	open := slices.Concat(d.GitHub.Findings, d.GitHub.Debt)
	age := ages(d.Now, open)

	flakyIssues := 0

	for _, i := range d.GitHub.Debt {
		if strings.Contains(strings.ToLower(i.Title), "flak") {
			flakyIssues++
		}
	}

	oldest := math.NaN()
	if len(age) > 0 {
		oldest = slices.Max(age)
	}

	findings := [][]string{}

	for _, i := range slices.SortedFunc(slices.Values(open), func(a, b Issue) int { return a.Created.Compare(b.Created) }) {
		findings = append(findings, []string{fmt.Sprintf("#%d", i.Number), i.Title, days(d.Now.Sub(i.Created).Hours() / 24)}) //nolint:mnd // hours in a day
	}

	return []Block{
		{Kind: kindTiles, Title: "DORA-style, over the window", Tiles: []Tile{
			{Label: "Deployment frequency", Value: fmt.Sprintf("%.1f", releasesPerMonth(d)), Note: "release tags a month"},
			{Label: "Lead time for changes", Value: days(median(lead)), Note: "median, first commit to merge"},
			{Label: "Change failure rate", Value: percent(failureRate(runs)), Note: "share of main's CI runs that failed"},
			{Label: "Time to restore", Value: hours(timeToRestore(runs)), Note: "median, a red main run to the next green one"},
		}},
		{Kind: kindTiles, Title: "Nightly, review, flaky, findings", Tiles: []Tile{
			{Label: "Nightly pass rate", Value: percent(hundred - failureRate(nightly)), Note: fmt.Sprintf("of %d nightly runs", len(nightly))},
			{Label: "Review cost", Value: thousands(median(tokens)), Note: fmt.Sprintf("median tokens, %d reviewed pull requests", len(tokens))},
			{Label: "Review yield", Value: decimal(yield, ""), Note: "findings fixed per 100k tokens"},
			{Label: "Flaky", Value: strconv.Itoa(flakyRuns(runs)), Note: fmt.Sprintf("main runs green only on a retry; %d open flaky-test issues", flakyIssues)},
			{Label: "Open findings", Value: strconv.Itoa(len(open)), Note: "nightly findings and debt, the oldest " + days(oldest)},
		}},
		{
			Kind: kindBars, Title: "Main CI failure rate a week",
			Items: weekly(d.Since, d.Now, runs, func(r Run) time.Time { return r.Created }, failureRate, percent),
		},
		{Kind: kindTable, Title: "Open findings and debt", Head: []string{"Issue", "Title", "Open for"}, Rows: findings},
	}
}

// decimal is a value with one decimal and its unit, "–" when there is none.
func decimal(v float64, unit string) string {
	if math.IsNaN(v) {
		return "–"
	}

	return fmt.Sprintf("%.1f%s", v, unit)
}

func hours(h float64) string {
	if math.IsNaN(h) {
		return "–"
	}

	return fmt.Sprintf("%.1f h", h)
}

func thousands(n float64) string {
	if math.IsNaN(n) {
		return "–"
	}

	return fmt.Sprintf("%.0fk", n/thousand)
}

package thresholds

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/triage"
)

// Label marks the issues the thresholds own: the nightly opens, comments and closes only those.
const Label = "metrics"

// LabelColor and LabelDescription describe the label when the first issue creates it.
const (
	LabelColor       = "FBCA04"
	LabelDescription = "A metric past its threshold in gates/metrics.yaml, filed and closed by bin/harness metrics"
)

// marker is how an issue names the threshold it tracks, in a comment GitHub does not render.
var marker = regexp.MustCompile(`<!-- metrics-threshold: (\S+) -->`)

// levelMarker is the level a drop's issue is measured from, kept in the issue for a night that
// cannot download the last night's values.
var levelMarker = regexp.MustCompile(`<!-- metrics-from: ([0-9.]+) -->`)

// levelOf is the level an issue says its drop is measured from; false for none.
func levelOf(i triage.Issue) (float64, bool) {
	m := levelMarker.FindStringSubmatch(i.Body)
	if m == nil {
		return 0, false
	}

	v, err := strconv.ParseFloat(m[1], 64)

	return v, err == nil
}

// thresholdOf is the threshold an issue tracks, or "".
func thresholdOf(i triage.Issue) string {
	if m := marker.FindStringSubmatch(i.Body); m != nil {
		return m[1]
	}

	return ""
}

// Plan is what tonight's outcomes do to the open metrics issues: a threshold crossed with no issue
// opens one, crossed with one comments on it, and back under its limit closes it. A metric that
// could not be measured comments on its issue, and opens none: the other findings say why. from
// holds, for each drop crossed, the level it is measured from: an issue it opens keeps it.
func Plan(c Config, outcomes []triage.Outcome, open []triage.Issue, runURL string, from map[string]float64) []triage.Action {
	why := map[string]string{}
	for _, t := range c.Thresholds {
		why[t.ID] = t.Why
	}

	byID := map[string]triage.Issue{}

	for _, i := range open {
		if id := thresholdOf(i); id != "" {
			byID[id] = i
		}
	}

	var actions []triage.Action

	for _, o := range outcomes {
		issue, tracked := byID[o.Gate]

		switch {
		case o.Verdict == runner.Fail && tracked:
			actions = append(actions, triage.Action{Kind: triage.Comment, Issue: issue.Number, Body: "Still past its threshold in " + runURL + ": " + o.Summary + "."})
		case o.Verdict == runner.Fail:
			level := ""
			if v, ok := from[o.Gate]; ok {
				level = fmt.Sprintf("<!-- metrics-from: %g -->\n", v)
			}

			actions = append(actions, triage.Action{
				Kind: triage.Open, Title: "metrics: " + o.Gate + " past its threshold",
				Body: fmt.Sprintf("<!-- metrics-threshold: %s -->\n%s%s, in %s.\n\nWhy it is watched: %s\n\n"+
					"The threshold is `%s` in `%s`. This issue closes on the first night the metric is back under it; `make dashboard` shows the metric over time.\n",
					o.Gate, level, o.Summary, runURL, why[o.Gate], o.Gate, Path),
			})
		case o.Verdict == runner.Unavailable && tracked:
			actions = append(actions, triage.Action{Kind: triage.Comment, Issue: issue.Number, Body: "Not measured in " + runURL + ": " + o.Summary + "."})
		case o.Verdict == runner.Pass && tracked:
			actions = append(actions, triage.Action{Kind: triage.Close, Issue: issue.Number, Body: "Back under its threshold in " + runURL + ": " + o.Summary + "."})
		}
	}

	// An issue whose threshold left gates/metrics.yaml (removed, or renamed) has nothing left to
	// hold it open.
	for _, id := range slices.Sorted(maps.Keys(byID)) {
		if _, kept := why[id]; !kept {
			actions = append(actions, triage.Action{Kind: triage.Close, Issue: byID[id].Number, Body: "Its threshold `" + id + "` is no longer in `" + Path + "`, as of " + runURL + "."})
		}
	}

	return actions
}

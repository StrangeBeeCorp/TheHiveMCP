package policy

import (
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/shell"
)

func (j *judge) gh(args []string) Decision {
	group, sub := "", ""
	if len(args) > 0 {
		group = args[0]
	}

	if len(args) > 1 {
		sub = args[1]
	}

	switch {
	case group == "pr" && sub == "merge":
		return deny("agents never merge: a person merges the pull request on GitHub")
	case group == "pr" && sub == "review" && slices.ContainsFunc(args, func(a string) bool { return a == "--approve" || a == "-a" }):
		return deny("agents never approve: a person reviews the pull request")
	case group == "api":
		return j.ghAPI(args[1:])
	case !j.role.Agent() || ghRead(group, sub):
		return Allow
	case j.role == Other && group == "pr" && slices.Contains([]string{"create", "edit", "comment"}, sub):
		return Allow
	}

	return deny("the %s may read GitHub with gh (view, list, diff, checks, status) but not change it", j.role)
}

func ghRead(group, sub string) bool {
	switch group {
	case "pr", "issue", "run", "release", "repo", "workflow", "label", "cache":
		return slices.Contains([]string{"view", subList, subDiff, "checks", subStatus, "watch"}, sub)
	case "search", subStatus, "browse", subHelp, subVersion:
		return true
	case "auth":
		return sub == subStatus
	}

	return false
}

// ghAPI allows reads. Agents never write; the main session never writes statuses, checks,
// reviews or merges, which would forge the evidence a person relies on, through REST or GraphQL.
func (j *judge) ghAPI(args []string) Decision {
	call := parseAPI(args)
	if call.method == http.MethodGet && !call.graphql() {
		return Allow
	}

	if j.role.Agent() {
		return deny("agents only read the GitHub API: use gh pr create, gh pr edit or gh pr comment")
	}

	switch {
	case slices.ContainsFunc(call.endpoints, func(e string) bool { return strings.Contains(e, shell.Computed) }):
		return deny("write gh api's endpoint out: the policy reads it before it runs")
	case call.graphql() && call.queryHidden:
		return deny("write the GraphQL query inline (-f query='…'): the policy reads it before it runs")
	case call.graphql() && forgesEvidence(strings.ToLower(call.query), "mutation", "merge", "review", "checkrun", "checksuite", "status"):
		return deny("statuses, checks, reviews and merges are what people rely on: nobody writes them from here")
	}

	for _, e := range call.endpoints {
		if forgesEvidence(e, "", "/statuses/", "/check-runs", "/check-suites", "/merge", "/reviews") {
			return deny("statuses, checks, reviews and merges are what people rely on: nobody writes them from here")
		}
	}

	return Allow
}

// forgesEvidence reports whether text, once it holds marker (any text for ""), names one of words.
func forgesEvidence(text, marker string, words ...string) bool {
	return strings.Contains(text, marker) && slices.ContainsFunc(words, func(w string) bool { return strings.Contains(text, w) })
}

// apiCall is what gh api's arguments ask: the method, the endpoints, and the GraphQL query.
type apiCall struct {
	method    string
	endpoints []string
	query     string
	// queryHidden is set when the request's body comes from a file or stdin (--input, -F x=@file).
	queryHidden bool
}

func (c apiCall) graphql() bool {
	return slices.ContainsFunc(c.endpoints, func(e string) bool { return e == "graphql" || strings.HasSuffix(e, "/graphql") })
}

// shellExpanded finds what the shell put in a word as the line runs: ${NAME}, or a bare $ that a
// GraphQL variable ($owner) never is.
var shellExpanded = regexp.MustCompile(`\$(\{|[^A-Za-z_]|$)`)

const (
	// apiInput sends a file, or stdin, as the request's body.
	apiInput = "--input"
	// apiRawField is a field sent as it is: its value never names a file.
	apiRawField = "--raw-field"
)

// apiValued are gh api's options that take a value; -i and the long booleans take none.
var apiValued = []string{
	"-X", "--method", "-f", apiRawField, "-F", "--field", "-H", "--header", apiInput, "-q", "--jq", "-t", "--template",
	"-p", "--preview", "--hostname", "--cache",
}

// parseAPI reads gh api's arguments as gh does: -XPUT and -X PUT, --method=PUT, short options in a
// cluster (-iXPUT), and the endpoint wherever it is.
func parseAPI(args []string) apiCall {
	var r apiReader

	for i := 0; i < len(args); i++ {
		a := args[i]

		switch {
		case a == "--":
			r.call.endpoints = append(r.call.endpoints, args[i+1:]...)
			i = len(args)
		case strings.HasPrefix(a, "-") && len(a) > 1:
			flag, value, next := apiOption(a)
			if next && i+1 < len(args) {
				i++
				value = args[i]
			}

			r.set(flag, value)
		default:
			r.call.endpoints = append(r.call.endpoints, a)
		}
	}

	return r.result()
}

// apiOption reads one option word: its flag (in a cluster, the first that takes a value), the
// value attached to it, and whether its value is the next word instead.
func apiOption(a string) (string, string, bool) {
	if strings.HasPrefix(a, "--") {
		flag, value, attached := strings.Cut(a, "=")
		return flag, value, !attached && slices.Contains(apiValued, flag)
	}

	for k := 1; k < len(a); k++ {
		if flag := "-" + a[k:k+1]; slices.Contains(apiValued, flag) {
			return flag, strings.TrimPrefix(a[k+1:], "="), k+1 == len(a)
		}
	}

	return a, "", false
}

// apiReader gathers what gh api's options ask.
type apiReader struct {
	call     apiCall
	explicit string
	fields   bool
}

func (r *apiReader) set(flag, value string) {
	switch flag {
	case "-X", "--method":
		r.explicit = strings.ToUpper(value)
	case "-f", apiRawField, "-F", "--field", apiInput:
		r.fields = true

		key, v, _ := strings.Cut(value, "=")
		raw := flag == "-f" || flag == apiRawField

		switch {
		case flag == apiInput || (key == "query" && !raw && strings.HasPrefix(v, "@")):
			r.call.queryHidden = true
		case key == "query":
			r.call.query += v
			r.call.queryHidden = r.call.queryHidden || shellExpanded.MatchString(v)
		}
	}
}

// result is the call, with its method: the one asked for, else POST with fields, else GET.
func (r *apiReader) result() apiCall {
	switch {
	case r.explicit != "":
		r.call.method = r.explicit
	case r.fields:
		r.call.method = http.MethodPost
	default:
		r.call.method = http.MethodGet
	}

	return r.call
}

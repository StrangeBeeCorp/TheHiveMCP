package policy

import (
	"net/http"
	"slices"
	"strings"
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
// reviews or merges, which would forge the evidence a person relies on.
func (j *judge) ghAPI(args []string) Decision {
	method, endpoint := apiCall(args)
	if method == http.MethodGet {
		return Allow
	}

	if j.role.Agent() {
		return deny("agents only read the GitHub API: use gh pr create, gh pr edit or gh pr comment")
	}

	for _, forged := range []string{"/statuses/", "/check-runs", "/check-suites", "/merge", "/reviews"} {
		if strings.Contains(endpoint, forged) {
			return deny("statuses, checks, reviews and merges are what people rely on: nobody writes them from here")
		}
	}

	return Allow
}

// apiCall reads the method and the endpoint of gh api's arguments. Fields make it a POST.
func apiCall(args []string) (string, string) {
	method, endpoint := http.MethodGet, ""

	for i, a := range args {
		switch {
		case (a == "-X" || a == "--method") && i+1 < len(args):
			method = strings.ToUpper(args[i+1])
		case strings.HasPrefix(a, "--method="):
			method = strings.ToUpper(strings.TrimPrefix(a, "--method="))
		case slices.Contains([]string{"-f", "-F", "--field", "--raw-field", "--input"}, a) && method == http.MethodGet:
			method = http.MethodPost
		case endpoint == "" && !strings.HasPrefix(a, "-") && (i == 0 || !strings.HasPrefix(args[i-1], "-")):
			endpoint = a
		}
	}

	return method, endpoint
}

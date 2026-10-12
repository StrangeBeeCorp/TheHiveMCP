package checks_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Renovate opens a fix pull request for a known CVE as docs/how-to/handle-a-cve.md says: from
// OSV, labelled security, with no release-age wait or schedule of its own (Renovate's defaults for
// vulnerability fixes), and the dashboard lists the CVEs with no fix yet. No gate reads
// renovate.json; this test, which every change runs, does.
func TestRenovateOpensAFixPullRequestForAKnownCVE(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(filepath.Join("..", "..", "..", "renovate.json"))
	require.NoError(t, err)

	var config struct {
		OSV       bool           `json:"osvVulnerabilityAlerts"`
		Dashboard string         `json:"dependencyDashboardOSVVulnerabilitySummary"`
		Alerts    map[string]any `json:"vulnerabilityAlerts"`
	}

	require.NoError(t, json.Unmarshal(data, &config))

	assert.True(t, config.OSV, "OSV's alerts open the fix pull requests")
	assert.Equal(t, "unresolved", config.Dashboard, "the dashboard lists the CVEs with no fix yet")
	assert.Equal(t, []any{"security"}, config.Alerts["labels"])

	for _, key := range []string{"minimumReleaseAge", "schedule", "prCreation", "dependencyDashboardApproval", "enabled"} {
		assert.NotContains(t, config.Alerts, key, "a fix pull request opens at once: Renovate's default, which %s would override", key)
	}
}

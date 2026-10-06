package load

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
)

// The thehive run of gates/load.yaml against the integration stack's TheHive (5.5 and 5.6 in CI):
// the server built and run as its HTTP users do, the test's own user's key sent by every client,
// each searching the 25 alerts seeded here, and the result held to the run's budget. It runs in
// the load-thehive gate, alone on the stack: inside the suite, whose tests run in parallel under a
// license, its 200 searches would slow the other tests' TheHive, and theirs its measurement.
func TestLoadAgainstTheHive(t *testing.T) {
	if os.Getenv(loadEnv) == "" {
		t.Skip("the load-thehive gate runs it alone on the stack (" + loadEnv + "=1)")
	}

	testutils.Parallel(t)

	hive := testutils.SetupTestWithCleanup(t)
	auth := testutils.GetAuthContext(t)

	for i := range stubAlerts {
		alert := testutils.MockInputAlert()
		alert.Title = fmt.Sprintf("Load alert %d", i)
		alert.SourceRef = fmt.Sprintf("load-%d", i)

		_, resp, err := hive.AlertAPI.CreateAlert(auth).InputCreateAlert(*alert).Execute()
		require.NoError(t, err)

		_ = resp.Body.Close()
	}

	org := hive.GetConfig().DefaultHeader["X-Organisation"]
	basic, ok := auth.Value(thehive.ContextBasicAuth).(thehive.BasicAuth)
	require.True(t, ok)

	key := renewKey(t, testutils.TheHiveTestURL(), org, basic)

	budgets, err := ReadBudgets(filepath.Join("..", "..", BudgetsPath))
	require.NoError(t, err)

	run, err := budgets.Run("thehive")
	require.NoError(t, err)

	bin, err := Build(t.Context(), filepath.Join("..", ".."), t.TempDir())
	require.NoError(t, err)

	var log strings.Builder

	r, err := Measure(t.Context(), "thehive", run.Config, bin, Target{URL: testutils.TheHiveTestURL(), Key: key, Org: org}, &log)
	require.NoError(t, err, log.String())
	t.Log(r.Summary())

	// The result is kept where a person takes the baseline from, and compared with the one there.
	root := filepath.Join("..", "..")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".cache", "harness", "load"), 0o750))

	image := strings.NewReplacer("/", "_", ":", "-").Replace(testutils.TheHiveTestImage())
	require.NoError(t, r.Write(filepath.Join(root, ".cache", "harness", "load", image+".json")))

	base, err := ReadResult(Baseline(root, "thehive"))
	if err == nil {
		t.Log(Compare(r, base))
	}

	assert.Empty(t, run.Budget.Check(r), "the thehive run is held to its budget in "+BudgetsPath)
}

// loadEnv is what the load-thehive gate sets to run the TheHive load.
const loadEnv = "HARNESS_LOAD"

// renewKey gives the test's user an API key: the server's clients send a key, as its users do.
func renewKey(t *testing.T, base, org string, user thehive.BasicAuth) string {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		strings.TrimRight(base, "/")+"/api/v1/user/"+url.PathEscape(user.UserName)+"/key/renew", nil)
	require.NoError(t, err)
	req.SetBasicAuth(user.UserName, user.Password)
	req.Header.Set("X-Organisation", org)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "renew the key: %s", body)

	return strings.Trim(strings.TrimSpace(string(body)), `"`)
}

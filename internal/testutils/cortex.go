package testutils

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// The Cortex of the compose stack (docker-compose.test.yml) runs two shell neurons with the
// process runner (testdata/cortex/neurons). RequireCortex bootstraps it and points TheHive's
// Cortex connector at it, as initHiveInstance imports ATT&CK from mitre-server.
const (
	// TestCortexResponder and TestCortexAnalyzer are the definition ids of the stack's neurons:
	// the responder answers success and acts on nothing, the analyzer gives one info verdict.
	TestCortexResponder = "TheHiveMCP_Test_Responder_1_0"
	TestCortexAnalyzer  = "TheHiveMCP_Test_Analyzer_1_0"

	// envTestCortexURL overrides where the test process reaches the stack's Cortex.
	envTestCortexURL = "THEHIVE_TEST_CORTEX_URL"
	cortexPort       = "9001"
	// cortexInTheHive is how TheHive reaches Cortex: over the compose network, by service name.
	cortexInTheHive = "http://cortex:" + cortexPort
	// cortexServerName names the server in TheHive's connector (the cortexIds a worker carries): the
	// MCP server's default Cortex ID, so a call without cortex-id reaches it.
	cortexServerName = "local"

	// The superadmin, the organisation and its user TheHive calls Cortex as. The user's login
	// must differ from the organisation's name: both are ids in one Cortex index.
	cortexSuperadmin = "admin"
	cortexOrg        = "thehivemcp"
	cortexUser       = "thehive"
	cortexPassword   = "secret" // #nosec G101 -- test fixture of the disposable stack, not a real secret

	cortexReadyTimeout = 5 * time.Minute
	cortexPoll         = 2 * time.Second

	// cortexNoDatabase is Cortex's answer before its database is migrated.
	cortexNoDatabase = 520
)

var (
	cortexOnce sync.Once
	errCortex  error
)

// CortexTestURL is where the test process reaches the stack's Cortex: THEHIVE_TEST_CORTEX_URL, or
// port 9001 of TheHive's host (the compose file publishes both).
func CortexTestURL() string {
	if u := os.Getenv(envTestCortexURL); u != "" {
		return u
	}

	return cortexBesides(TheHiveTestURL())
}

// cortexBesides is the Cortex URL on the host of a TheHive URL.
func cortexBesides(theHiveURL string) string {
	host := "localhost"

	parsed, err := url.Parse(theHiveURL)
	if err == nil && parsed.Hostname() != "" {
		host = parsed.Hostname()
	}

	return "http://" + net.JoinHostPort(host, cortexPort)
}

// RequireCortex makes the stack's Cortex serve TheHive before a test runs an analyzer or a
// responder: it bootstraps Cortex (superadmin, organisation, user, the two neurons enabled) and
// sets TheHive's Cortex connector, once per test binary; every step is idempotent, so the packages
// that run at the same time agree. A failure fails the test: the stack has a Cortex, and a test
// that skipped without it would prove nothing.
//
// In shared mode it changes nothing: the connector is instance-wide, and the shared TheHive's own
// Cortex serves the test, if it has one.
func RequireCortex(t *testing.T) {
	t.Helper()

	hive, err := StartTheHiveContainer(t)
	if err != nil {
		t.Fatalf("%v", err)
	}

	if SharedMode() {
		return
	}

	cortexOnce.Do(func() {
		errCortex = newCortexWiring(hive).wire(context.Background())
	})

	if errCortex != nil {
		t.Fatalf("the integration stack's Cortex: %v", errCortex)
	}
}

// cortexWiring bootstraps one Cortex and points one TheHive at it.
type cortexWiring struct {
	cortex, theHive string
	// theHiveUser and theHivePassword are a superadmin's: the connector is global configuration.
	theHiveUser, theHivePassword string
	client                       *http.Client
	ready                        time.Duration
	poll                         time.Duration
}

func newCortexWiring(theHive string) cortexWiring {
	return cortexWiring{
		cortex: CortexTestURL(), theHive: theHive,
		theHiveUser: adminUser(), theHivePassword: adminPassword(),
		client: &http.Client{Timeout: 30 * time.Second},
		ready:  cortexReadyTimeout, poll: cortexPoll,
	}
}

// errCortexAnswer is a Cortex or TheHive answer the bootstrap cannot go on from.
var errCortexAnswer = errors.New("unexpected answer")

// wire runs the whole bootstrap.
func (w cortexWiring) wire(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, w.ready)
	defer cancel()

	steps := []struct {
		what string
		run  func(context.Context) error
	}{
		{"create its superadmin", w.superadmin},
		{"create its organisation and user", w.orgUser},
		{"enable the test neurons", w.neurons},
		{"set TheHive's Cortex connector", w.connector},
	}

	for _, step := range steps {
		err := step.run(ctx)
		if err != nil {
			return fmt.Errorf("%s at %s: %w", step.what, w.cortex, err)
		}
	}

	return nil
}

// superadmin waits for Cortex, migrates its database and creates the superadmin, whichever of
// these an earlier run left to do. Cortex tells where it stands by how it answers an anonymous
// GET /api/user/current: 520 no database yet, 404 no user yet, 401 users exist.
func (w cortexWiring) superadmin(ctx context.Context) error {
	for {
		status, body, err := w.call(ctx, w.cortex, http.MethodGet, "/api/user/current", "", "", nil)

		switch {
		case err != nil || status >= http.StatusInternalServerError && status != cortexNoDatabase:
			// not up yet: a refused connection or a 5xx of a starting Play application
		case status == http.StatusUnauthorized:
			return nil
		case status == cortexNoDatabase:
			_, _, _ = w.call(ctx, w.cortex, http.MethodPost, "/api/maintenance/migrate", "", "", nil)
		case status == http.StatusNotFound:
			err = w.expect(ctx, w.cortex, http.MethodPost, "/api/user", "", "", cortexUserBody{
				Login: cortexSuperadmin, Name: cortexSuperadmin, Password: cortexPassword, Roles: []string{"superadmin"}, Organization: "cortex",
			})
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w: GET /api/user/current answered %d: %s", errCortexAnswer, status, body)
		}

		err = w.sleep(ctx)
		if err != nil {
			return err
		}
	}
}

// orgUser creates the organisation and its user, and sets the user's password.
func (w cortexWiring) orgUser(ctx context.Context) error {
	err := w.expect(ctx, w.cortex, http.MethodPost, "/api/organization", cortexSuperadmin, cortexPassword, cortexOrgBody{
		Name: cortexOrg, Description: "TheHiveMCP integration suite", Status: "Active",
	})
	if err == nil {
		err = w.expect(ctx, w.cortex, http.MethodPost, "/api/user", cortexSuperadmin, cortexPassword, cortexUserBody{
			Login: cortexUser, Name: "TheHive", Organization: cortexOrg, Roles: []string{"read", "analyze", "orgadmin"},
		})
	}

	if err == nil {
		err = w.expect(ctx, w.cortex, http.MethodPost, "/api/user/"+cortexUser+"/password/set", cortexSuperadmin, cortexPassword,
			cortexPasswordBody{Password: cortexPassword})
	}

	return err
}

// The bodies the bootstrap sends to Cortex and TheHive.
type (
	cortexUserBody struct {
		Login        string   `json:"login"`
		Name         string   `json:"name"`
		Password     string   `json:"password,omitempty"`
		Roles        []string `json:"roles"`
		Organization string   `json:"organization"`
	}
	cortexOrgBody struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Status      string `json:"status"`
	}
	cortexPasswordBody struct {
		Password string `json:"password"`
	}
	cortexWorkerBody struct {
		Name          string            `json:"name"`
		Configuration map[string]string `json:"configuration"`
		JobCache      int               `json:"jobCache"`
	}
	// theHiveConnectorBody is TheHive's cortex.servers configuration item.
	theHiveConnectorBody struct {
		Value []theHiveCortexServer `json:"value"`
	}
	theHiveCortexServer struct {
		Name string          `json:"name"`
		URL  string          `json:"url"`
		Auth theHiveAuthBody `json:"auth"`
	}
	theHiveAuthBody struct {
		Type     string `json:"type"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
)

// neurons enables the test analyzer and responder in the organisation, once Cortex has read them
// from its catalogs (it does, asynchronously, after it starts).
func (w cortexWiring) neurons(ctx context.Context) error {
	for _, worker := range []struct{ kind, id string }{{"analyzer", TestCortexAnalyzer}, {"responder", TestCortexResponder}} {
		err := w.awaitDefinition(ctx, worker.kind, worker.id)
		if err != nil {
			return err
		}

		err = w.expect(ctx, w.cortex, http.MethodPost, "/api/organization/"+worker.kind+"/"+worker.id, cortexUser, cortexPassword,
			cortexWorkerBody{Name: worker.id, Configuration: map[string]string{}})
		if err != nil {
			return err
		}
	}

	return nil
}

// awaitDefinition waits until Cortex lists the definition of id among its kind's.
func (w cortexWiring) awaitDefinition(ctx context.Context, kind, id string) error {
	for {
		status, body, err := w.call(ctx, w.cortex, http.MethodGet, "/api/"+kind+"definition", cortexUser, cortexPassword, nil)
		if err == nil && status == http.StatusOK && bytes.Contains(body, []byte(`"`+id+`"`)) {
			return nil
		}

		err = w.sleep(ctx)
		if err != nil {
			return fmt.Errorf("no %s definition %s in the catalog it was given: %w", kind, id, err)
		}
	}
}

// connector sets TheHive's Cortex connector to this Cortex, as the organisation's user. TheHive
// keeps it in its database, which the harness resets before each run.
func (w cortexWiring) connector(ctx context.Context) error {
	return w.expect(ctx, w.theHive, http.MethodPut, "/api/v1/config/cortex.servers", w.theHiveUser, w.theHivePassword, theHiveConnectorBody{
		Value: []theHiveCortexServer{{
			Name: cortexServerName, URL: cortexInTheHive,
			Auth: theHiveAuthBody{Type: "basic", Username: cortexUser, Password: cortexPassword},
		}},
	})
}

// expect sends a request that must succeed. Cortex's ConflictError, the answer to a create of what
// already exists, counts as a success: a rerun finds the stack bootstrapped.
func (w cortexWiring) expect(ctx context.Context, base, method, path, user, password string, payload any) error {
	status, body, err := w.call(ctx, base, method, path, user, password, payload)
	if err != nil {
		return err
	}

	if status >= http.StatusOK && status < http.StatusMultipleChoices || bytes.Contains(body, []byte(`"ConflictError"`)) {
		return nil
	}

	return fmt.Errorf("%w: %s %s answered %d: %s", errCortexAnswer, method, path, status, strings.TrimSpace(string(body)))
}

// call sends one request, with basic authentication when user is set, and returns the answer.
func (w cortexWiring) call(ctx context.Context, base, method, path, user, password string, payload any) (int, []byte, error) {
	var body io.Reader = http.NoBody

	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, fmt.Errorf("encode %s %s: %w", method, path, err)
		}

		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, base+path, body)
	if err != nil {
		return 0, nil, fmt.Errorf("build %s %s: %w", method, path, err)
	}

	// Cortex answers 400 to a JSON content type with no body, so it goes only with one.
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if user != "" {
		req.SetBasicAuth(user, password)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%s %s: %w", method, path, err)
	}

	defer func() { _ = resp.Body.Close() }()

	answer, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read %s %s: %w", method, path, err)
	}

	return resp.StatusCode, answer, nil
}

func (w cortexWiring) sleep(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("not ready in time: %w", ctx.Err())
	case <-time.After(w.poll):
		return nil
	}
}

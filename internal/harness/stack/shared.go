package stack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/proc"
)

// The shared mode runs the suite, one test at a time, inside ONE organisation of an
// already-running TheHive (the devstack one, typically) instead of the disposable compose stack:
// no stack to start, no database reset, no license, no platform admin, nothing instance-wide. The
// suite authenticates with the API key of an org-admin of that organisation and purges the
// organisation after every test (internal/testutils). It is opt-in: THEHIVE_TEST_SHARED, which
// make test-integration-shared sets.

// DefaultSharedOrg is the organisation leased when THEHIVE_TEST_ORG names none.
const DefaultSharedOrg = "thehivemcp-it"

// leaseReason is what the lease says it is for, on devstack's dashboard.
const leaseReason = "TheHiveMCP integration tests"

// releaseWait bounds the release of the lease, which must run even after the run was interrupted.
const releaseWait = time.Minute

// ErrNoTarget is no way to find the shared TheHive: nothing supplied, and no devstack to ask.
var ErrNoTarget = errors.New("shared mode: no TheHive to run in")

// WantsShared reports whether THEHIVE_TEST_SHARED asks for the shared mode; the suite reads the
// same values (testutils.SharedMode).
func WantsShared(value string) bool {
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// Shared finds the TheHive and the organisation the shared mode runs in.
type Shared struct {
	// URL, Org, Key and KeyFile are THEHIVE_TEST_URL, THEHIVE_TEST_ORG, THEHIVE_TEST_API_KEY and
	// THEHIVE_TEST_API_KEY_FILE (a file holding the key). With a key (or a key file) the target is
	// what was supplied; without, an organisation is leased from devstack (Org, or DefaultSharedOrg),
	// and URL, when set, still overrides the URL of the lease.
	URL, Org, Key, KeyFile string
	// Run captures a command's output; Look finds a command on the PATH; ReadFile reads the key file.
	Run      proc.Exec
	Look     func(name string) (string, error)
	ReadFile func(path string) ([]byte, error)
	Log      io.Writer
}

// Target is where the suite runs in the shared mode.
type Target struct {
	URL, Org, Key string
}

// lease is what `devstack lease thehive` prints.
type lease struct {
	ID        string `json:"lease"`
	URL       string `json:"thehive_url_from_containers"`
	Org       string `json:"org"`
	TokenFile string `json:"token_file"`
}

// Acquire resolves the target, and returns what releases it: always callable, and to be run
// whatever the suite did, since a lease holds the organisation until it is released or expires.
func (s Shared) Acquire(ctx context.Context) (Target, func(), error) {
	nothing := func() {}

	if s.Key == "" && s.KeyFile == "" {
		return s.lease(ctx)
	}

	key := strings.TrimSpace(s.Key)
	if s.KeyFile != "" {
		data, err := s.ReadFile(s.KeyFile)
		if err != nil {
			return Target{}, nothing, fmt.Errorf("shared mode: read the API key file: %w", err)
		}

		key = strings.TrimSpace(string(data))
	}

	t := Target{URL: s.URL, Org: s.Org, Key: key}
	if t.URL == "" || t.Org == "" || t.Key == "" {
		return Target{}, nothing, fmt.Errorf("%w: with an API key, THEHIVE_TEST_URL and THEHIVE_TEST_ORG must be set too", ErrNoTarget)
	}

	return t, nothing, nil
}

// lease asks devstack for an organisation of its shared TheHive.
func (s Shared) lease(ctx context.Context) (Target, func(), error) {
	nothing := func() {}

	_, err := s.Look("devstack")
	if err != nil {
		return Target{}, nothing, fmt.Errorf("%w: set THEHIVE_TEST_URL, THEHIVE_TEST_ORG and THEHIVE_TEST_API_KEY_FILE "+
			"(a file holding that organisation's org-admin API key), or install devstack", ErrNoTarget)
	}

	org := s.Org
	if org == "" {
		org = DefaultSharedOrg
	}

	out, err := s.Run(ctx, []string{"devstack", "lease", "thehive", "--org", org, "--for", leaseReason}, nil)
	if err != nil {
		return Target{}, nothing, fmt.Errorf("shared mode: lease an organisation: %w", err)
	}

	if out.Code != 0 {
		return Target{}, nothing, fmt.Errorf("shared mode: lease an organisation: exit %d: %s", out.Code, strings.TrimSpace(out.Text()))
	}

	var l lease

	err = json.Unmarshal(out.Stdout, &l)
	if err != nil || l.ID == "" {
		return Target{}, nothing, fmt.Errorf("shared mode: devstack lease printed no lease: %s", strings.TrimSpace(out.Text()))
	}

	release := func() {
		// Not the run's context: an interrupted run must still give the organisation back.
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseWait)
		defer cancel()

		res, err := s.Run(rctx, []string{"devstack", "release", l.ID}, nil)
		if err != nil || res.Code != 0 {
			_, _ = fmt.Fprintf(s.Log, "the lease %s could not be released (it expires on its own): %v %s\n", l.ID, err, strings.TrimSpace(res.Text()))
		}
	}

	key, err := s.ReadFile(l.TokenFile)
	if err != nil {
		release()

		return Target{}, nothing, fmt.Errorf("shared mode: read the lease's API key file: %w", err)
	}

	t := Target{URL: l.URL, Org: l.Org, Key: strings.TrimSpace(string(key))}
	if s.URL != "" {
		t.URL = s.URL
	}

	_, _ = fmt.Fprintf(s.Log, "Leased organisation %s (lease %s).\n", t.Org, l.ID)

	return t, release, nil
}

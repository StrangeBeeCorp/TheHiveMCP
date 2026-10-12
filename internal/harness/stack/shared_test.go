package stack_test

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/proc"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/stack"
)

const (
	leaseFile = "/run/lease/token"
	keyPath   = "/keys/it"
	mount     = "/work:/app"
	goImage   = "golang:1.26.9"
	leaseJSON = `{"lease": "l-42", "thehive_url_from_containers": "http://thehive.devstack.orb.local:9000", "org": "thehivemcp-it", "token_file": "/run/lease/token"}`
)

// devstack answers the command lines of the shared mode, and records them.
type devstack struct {
	lease      string // what `devstack lease` prints
	leaseCode  int
	releaseErr error
	files      map[string]string
	calls      []string
}

func (d *devstack) run(_ context.Context, argv []string, _ io.Reader) (proc.Output, error) {
	d.calls = append(d.calls, strings.Join(argv, " "))

	switch argv[1] {
	case "lease":
		return proc.Output{Stdout: []byte(d.lease), Code: d.leaseCode}, nil
	case "release":
		return proc.Output{}, d.releaseErr
	}

	return proc.Output{Code: 2}, nil
}

func (d *devstack) shared() stack.Shared {
	return stack.Shared{
		Run:  d.run,
		Look: func(string) (string, error) { return "/usr/local/bin/devstack", nil },
		ReadFile: func(path string) ([]byte, error) {
			if content, ok := d.files[path]; ok {
				return []byte(content), nil
			}

			return nil, os.ErrNotExist
		},
		Log: io.Discard,
	}
}

// Only the values the suite reads as true turn the shared mode on.
func TestWantsSharedIsOptIn(t *testing.T) {
	t.Parallel()

	for value, want := range map[string]bool{"": false, "0": false, "false": false, "no": false, "1": true, "true": true, "TRUE": true, "yes": true, "on": true} {
		assert.Equal(t, want, stack.WantsShared(value), "THEHIVE_TEST_SHARED=%q", value)
	}
}

// With no key, an organisation is leased from devstack (the default one unless named), its key is
// read from the lease's file, and releasing it gives the lease back.
func TestAcquireLeasesAnOrganisationFromDevstack(t *testing.T) {
	t.Parallel()

	d := &devstack{lease: leaseJSON, files: map[string]string{leaseFile: "org-admin-key\n"}}

	target, release, err := d.shared().Acquire(t.Context())
	require.NoError(t, err)
	assert.Equal(t, stack.Target{URL: "http://thehive.devstack.orb.local:9000", Org: "thehivemcp-it", Key: "org-admin-key"}, target)
	assert.Equal(t, []string{"devstack lease thehive --org thehivemcp-it --for TheHiveMCP integration tests"}, d.calls)

	release()
	assert.Equal(t, "devstack release l-42", d.calls[len(d.calls)-1])

	// A named organisation is the one leased, and a supplied URL replaces the lease's.
	d = &devstack{lease: leaseJSON, files: map[string]string{leaseFile: "k"}}
	s := d.shared()
	s.Org, s.URL = "my-org", "http://localhost:9100"

	target, release, err = s.Acquire(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:9100", target.URL)
	assert.Equal(t, "devstack lease thehive --org my-org --for TheHiveMCP integration tests", d.calls[0])
	release()
}

// A key (or a file holding it) is a target supplied by hand: nothing is leased, so nothing is
// released, and the URL and the organisation must come with it.
func TestAcquireTakesASuppliedTarget(t *testing.T) {
	t.Parallel()

	d := &devstack{files: map[string]string{keyPath: " from-file \n"}}

	s := d.shared()
	s.URL, s.Org, s.Key = "http://host:9000", "org-1", "inline-key"

	target, release, err := s.Acquire(t.Context())
	require.NoError(t, err)
	assert.Equal(t, stack.Target{URL: "http://host:9000", Org: "org-1", Key: "inline-key"}, target)
	release()

	s.Key, s.KeyFile = "", keyPath
	target, _, err = s.Acquire(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "from-file", target.Key)

	s.KeyFile = "/keys/missing"
	_, _, err = s.Acquire(t.Context())
	require.ErrorContains(t, err, "read the API key file")

	s.KeyFile, s.Org = keyPath, ""
	_, _, err = s.Acquire(t.Context())
	require.ErrorIs(t, err, stack.ErrNoTarget)

	assert.Empty(t, d.calls, "devstack is not asked when the target is supplied")
}

// Without devstack and without a key there is no TheHive to run in, and the error says what to set.
func TestAcquireWithoutDevstackSaysWhatToSet(t *testing.T) {
	t.Parallel()

	d := &devstack{}
	s := d.shared()
	s.Look = func(string) (string, error) { return "", errors.New("not found") }

	_, _, err := s.Acquire(t.Context())
	require.ErrorIs(t, err, stack.ErrNoTarget)
	require.ErrorContains(t, err, "THEHIVE_TEST_API_KEY_FILE")
	assert.Empty(t, d.calls)
}

// A lease that fails, or prints no lease, stops the run; one whose key cannot be read is given back.
func TestAcquireFailuresLeaveNoLeaseBehind(t *testing.T) {
	t.Parallel()

	d := &devstack{lease: "no capacity", leaseCode: 1}
	_, release, err := d.shared().Acquire(t.Context())
	require.ErrorContains(t, err, "lease an organisation: exit 1: no capacity")
	release()
	assert.Len(t, d.calls, 1, "nothing to release")

	d = &devstack{lease: "not json"}
	_, _, err = d.shared().Acquire(t.Context())
	require.ErrorContains(t, err, "printed no lease")

	d = &devstack{lease: leaseJSON}
	_, release, err = d.shared().Acquire(t.Context())
	require.ErrorContains(t, err, "read the lease's API key file")
	release()
	assert.Equal(t, "devstack release l-42", d.calls[len(d.calls)-1], "the lease taken is released")
	assert.Len(t, d.calls, 2, "released once, by Acquire; the returned function has nothing left to do")
}

// A release that fails is reported, not fatal: the lease expires on its own.
func TestReleaseFailureIsOnlyReported(t *testing.T) {
	t.Parallel()

	var log strings.Builder

	d := &devstack{lease: leaseJSON, releaseErr: errors.New("devstack is gone"), files: map[string]string{leaseFile: "k"}}
	s := d.shared()
	s.Log = &log

	_, release, err := s.Acquire(t.Context())
	require.NoError(t, err)
	release()
	assert.Contains(t, log.String(), "lease l-42 could not be released")
}

// The release runs even when the run's own context is already cancelled (an interrupted run).
func TestReleaseSurvivesACancelledRun(t *testing.T) {
	t.Parallel()

	d := &devstack{lease: leaseJSON, files: map[string]string{leaseFile: "k"}}
	s := d.shared()

	var releaseCtxErr error

	s.Run = func(ctx context.Context, argv []string, stdin io.Reader) (proc.Output, error) {
		if argv[1] == "release" {
			releaseCtxErr = ctx.Err()
		}

		return d.run(ctx, argv, stdin)
	}

	ctx, cancel := context.WithCancel(t.Context())
	_, release, err := s.Acquire(ctx)
	require.NoError(t, err)

	cancel()
	release()
	require.NoError(t, releaseCtxErr, "the release does not inherit the cancellation")
	assert.Equal(t, "devstack release l-42", d.calls[len(d.calls)-1])
}

// The shared mode hands the suite its target by value, runs it one package at a time whatever the
// license, and leaves the default command line as it was.
func TestTestsArgvCarriesTheSharedTarget(t *testing.T) {
	t.Parallel()

	tests := stack.Tests{Mount: mount, Image: goImage, Run: []string{"-run", "TestA"}}
	plain := tests.Argv("eyJ.lic")

	tests.Shared = &stack.Target{URL: "http://thehive.devstack.orb.local:9000", Org: "thehivemcp-it", Key: "org-admin-key"}
	shared := tests.Argv("eyJ.lic")

	assert.Equal(t, plain[:10], shared[:10], "the same docker flags")
	assert.Equal(t, []string{
		"-e", "THEHIVE_TEST_URL=http://thehive.devstack.orb.local:9000", "-e", "LOG_LEVEL", "-e", "THEHIVE_TEST_IMAGE", "-e", "HARNESS_LOAD",
		"-e", "THEHIVE_TEST_LICENSE=", "-e", "CGO_ENABLED=0",
		"-e", "THEHIVE_TEST_SHARED=1", "-e", "THEHIVE_TEST_ORG=thehivemcp-it", "-e", "THEHIVE_TEST_API_KEY=org-admin-key",
		goImage, "sh", "-c",
	}, shared[10:len(shared)-1])
	assert.Contains(t, shared[len(shared)-1], "-run TestA -p 1 -timeout 20m ./...", "one package at a time, even with a license")

	assert.Contains(t, plain, "THEHIVE_TEST_LICENSE=eyJ.lic")
	assert.NotContains(t, strings.Join(plain, " "), "THEHIVE_TEST_SHARED")
	assert.NotContains(t, strings.Join(plain, " "), "THEHIVE_TEST_API_KEY")
	assert.Contains(t, plain[len(plain)-1], "-run TestA -timeout 20m ./...", "the default run with a license stays parallel")
}

// The variables named in Pass reach the container by name; none is a command line as it was.
func TestTestsArgvPassesTheNamedVariables(t *testing.T) {
	t.Parallel()

	tests := stack.Tests{Mount: mount, Image: goImage}
	before := tests.Argv("")

	tests.Pass = []string{"THEHIVE_TEST_ADMIN_USER", "THEHIVE_TEST_ADMIN_PASSWORD"}
	after := tests.Argv("")

	assert.Len(t, after, len(before)+4)
	assert.Contains(t, strings.Join(after, " "), "-e CGO_ENABLED=0 -e THEHIVE_TEST_ADMIN_USER -e THEHIVE_TEST_ADMIN_PASSWORD")
}

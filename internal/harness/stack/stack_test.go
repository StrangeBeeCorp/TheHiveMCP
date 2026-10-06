package stack_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/proc"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/stack"
)

const (
	licenseFile = "internal/testutils/testdata/.test-license.lic.local"
	base        = "docker compose -f docker-compose.test.yml"
	licensed    = base + " -f docker-compose.license.yml"
	minted      = "eyJ.minted.token"
	up          = " up -d"
	down        = " down -v"
)

// docker answers the command lines of a stack run: which licensing images are pullable, what the
// minter prints, which compose step fails; and records them all, in order.
type docker struct {
	pullable map[string]bool
	minter   string
	failing  string
	ready    int // tries before TheHive answers; -1: never
	calls    []string
	license  string // the license file when the stack came up
	root     string
}

func (d *docker) exec(_ context.Context, argv []string, _ io.Reader) (proc.Output, error) {
	line := strings.Join(argv, " ")
	d.calls = append(d.calls, line)

	switch {
	case strings.HasPrefix(line, "docker manifest inspect "):
		if d.pullable[argv[3]] {
			return proc.Output{}, nil
		}

		return proc.Output{Stderr: []byte("denied: not authorized"), Code: 1}, nil
	case strings.Contains(line, "--key-id dev"):
		return proc.Output{Stdout: []byte(d.minter)}, nil
	case strings.HasSuffix(line, up):
		data, _ := os.ReadFile(filepath.Join(d.root, licenseFile))
		d.license = string(data)
	}

	if d.failing != "" && strings.Contains(line, d.failing) {
		return proc.Output{Code: 1}, nil
	}

	return proc.Output{}, nil
}

func (d *docker) isReady(context.Context) bool {
	if d.ready < 0 {
		return false
	}

	d.ready--

	return d.ready < 0
}

func newStack(t *testing.T, d *docker) stack.Stack {
	t.Helper()

	d.root = t.TempDir()

	return stack.Stack{
		Root: d.root, Compose: "docker-compose.test.yml", LicenseOverride: "docker-compose.license.yml", LicenseFile: licenseFile,
		TheHiveImage: "strangebee/thehive:5.5.2", URL: "http://localhost:9000", DevUser: "admin", DevPassword: "secret",
		Run: d.exec, Stream: d.exec, Ready: d.isReady, Wait: time.Second, Poll: time.Millisecond, Log: io.Discard,
	}
}

func (d *docker) composeCalls() []string {
	var out []string

	for _, c := range d.calls {
		if strings.HasPrefix(c, "docker compose ") {
			out = append(out, c)
		}
	}

	return out
}

var resetSteps = []string{
	" exec -T cassandra cqlsh -e DROP KEYSPACE IF EXISTS thehive;",
	` exec -T elasticsearch sh -c curl -s -o /dev/null -X DELETE "http://localhost:9200/thehive*"`,
	" restart thehive",
}

func withReset(prefix string, first ...string) []string {
	out := append([]string{}, first...)
	for _, s := range resetSteps {
		out = append(out, prefix+s)
	}

	return out
}

// A supplied license is written before the stack starts, with the override that mounts it, and no
// image is probed.
func TestPrepareUsesASuppliedLicense(t *testing.T) {
	t.Parallel()

	d := &docker{}
	s := newStack(t, d)
	s.SuppliedLicense = "eyJ.supplied"

	mode, token, err := s.Prepare(t.Context())
	require.NoError(t, err)
	assert.Equal(t, stack.Supplied, mode)
	assert.Equal(t, "eyJ.supplied", token)
	assert.Equal(t, "eyJ.supplied", d.license, "TheHive reads it at boot")

	info, err := os.Stat(filepath.Join(d.root, licenseFile))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm(), "TheHive's user reads it through a read-only mount")
	assert.Equal(t, withReset(licensed, licensed+up), d.calls)
}

// The TheHive image's licensing tag is tried, then the fallback; the --dev TheHive is waited for,
// the license minted and written where the override mounts it.
func TestPrepareMintsALicenseWhenTheImageIsPullable(t *testing.T) {
	t.Parallel()

	d := &docker{pullable: map[string]bool{"ghcr.io/strangebee/tools/licensing:release-5.6": true}, minter: "log\nToken: " + minted + "\nOK\n", ready: 3}
	s := newStack(t, d)

	mode, token, err := s.Prepare(t.Context())
	require.NoError(t, err)
	assert.Equal(t, stack.Minted, mode)
	assert.Equal(t, minted, token)
	assert.Empty(t, d.license, "an empty file boots TheHive in --dev mode, for the mint")

	written, err := os.ReadFile(filepath.Join(d.root, licenseFile))
	require.NoError(t, err)
	assert.Equal(t, minted, string(written))

	assert.Equal(t, "docker manifest inspect ghcr.io/strangebee/tools/licensing:release-5.5", d.calls[0])
	assert.Equal(t, "docker manifest inspect ghcr.io/strangebee/tools/licensing:release-5.6", d.calls[1])
	assert.Equal(t, withReset(licensed, licensed+up), d.composeCalls())
}

// No pullable image is the free mode: no override, no license file, and the old one removed.
func TestPrepareRunsFreeWithoutALicense(t *testing.T) {
	t.Parallel()

	d := &docker{}
	s := newStack(t, d)
	require.NoError(t, os.MkdirAll(filepath.Join(d.root, filepath.Dir(licenseFile)), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(d.root, licenseFile), []byte("eyJ.stale"), 0o600))

	mode, token, err := s.Prepare(t.Context())
	require.NoError(t, err)
	assert.Equal(t, stack.Free, mode)
	assert.Empty(t, token)
	assert.NoFileExists(t, filepath.Join(d.root, licenseFile), "a stale token must not leak into this run")
	assert.Equal(t, withReset(base, base+up), d.composeCalls())
}

// A TheHive that never answers, a minter that prints no token and a failing step each stop the
// run with what went wrong, and tear the stack down.
func TestPrepareTearsTheStackDownOnAFailure(t *testing.T) {
	t.Parallel()

	pullable := map[string]bool{"ghcr.io/strangebee/tools/licensing:release-5.5": true}

	d := &docker{pullable: pullable, ready: -1}
	_, _, err := newStack(t, d).Prepare(t.Context())
	require.ErrorIs(t, err, stack.ErrNotReady)
	assert.Equal(t, []string{licensed + up, licensed + down}, d.composeCalls(), "no mint against a TheHive that is not there")

	d = &docker{pullable: pullable, minter: "Error: 401 Unauthorized\n"}
	_, _, err = newStack(t, d).Prepare(t.Context())
	require.ErrorContains(t, err, "no token (exit 0); the minter printed:\nError: 401 Unauthorized")
	assert.Equal(t, licensed+down, d.calls[len(d.calls)-1])

	d = &docker{failing: "cqlsh"}
	_, _, err = newStack(t, d).Prepare(t.Context())
	require.ErrorContains(t, err, "drop the keyspace: exit 1")
	assert.Equal(t, []string{base + up, base + resetSteps[0], base + down}, d.composeCalls())
}

// With a license the suite runs in parallel; without, one package at a time. A RUN regexp reaches
// go test whole through sh.
func TestTestsArgvRunsTheSuiteOnTheHostNetwork(t *testing.T) {
	t.Parallel()

	tests := stack.Tests{
		Mount: "/repo:/app", Image: "golang:1.26.8", Caches: []string{"-v", "thehivemcp-gomod:/go/pkg/mod"},
		Tools: []string{"-v", "thehivemcp-go-tools:/go/bin"}, Run: []string{"-run", "TestA|TestB"},
	}

	licensedRun := tests.Argv("eyJ.lic")
	assert.Equal(t, []string{
		"docker", "run", "-i", "--rm", "--network", "host", "-v", "/repo:/app", "-w", "/app",
		"-e", "THEHIVE_TEST_URL", "-e", "LOG_LEVEL", "-e", "THEHIVE_TEST_IMAGE", "-e", "HARNESS_LOAD", "-e", "THEHIVE_TEST_LICENSE=eyJ.lic", "-e", "CGO_ENABLED=0",
		"-v", "thehivemcp-gomod:/go/pkg/mod", "-v", "thehivemcp-go-tools:/go/bin", "golang:1.26.8", "sh", "-c",
	}, licensedRun[:len(licensedRun)-1])
	assert.Equal(t, "command -v gotestsum >/dev/null 2>&1 || go install gotest.tools/gotestsum@v1.13.0 ; "+
		"gotestsum --format pkgname --hide-summary=skipped -- -run 'TestA|TestB' -timeout 20m ./...", licensedRun[len(licensedRun)-1])

	free := tests.Argv("")
	assert.Contains(t, free[len(free)-1], "-run 'TestA|TestB' -p 1 -timeout 20m ./...", "one package at a time on the shared org")
	assert.Contains(t, free, "THEHIVE_TEST_LICENSE=")
}

package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/proc"
)

const hostTarget = runtime.GOOS + "-" + runtime.GOARCH

// releaseRepo is a checkout with a built binary for linux-amd64 and for this machine, and the
// MCPB icon.
func releaseRepo(t *testing.T) string {
	t.Helper()

	dir := gittest.Repo(t, map[string]string{gitignore: "build/\ndist/\n", "docs/images/theHivelogo.png": "png"})
	// Two writes: on linux/amd64, CI's platform, the host binary is the linux-amd64 one.
	gittest.Write(t, dir, map[string]string{"build/thehivemcp-linux-amd64": "elf"})
	gittest.Write(t, dir, map[string]string{"build/thehivemcp-" + hostTarget: "elf"})
	t.Chdir(dir)

	return dir
}

// fakePacker stands in for docker: it records the streamed command lines, and packs an MCPB by
// copying the workspace's manifest; exit is the code of any other command.
func fakePacker(t *testing.T, exit int) *[][]string {
	t.Helper()

	var calls [][]string

	prevStream, prevExec, prevReady := streamExec, toolExec, readyCheck
	fake := func(_ context.Context, argv []string, _ io.Reader) (proc.Output, error) {
		calls = append(calls, argv)

		if argv[len(argv)-3] == "pack" {
			ws := strings.TrimSuffix(argv[6], ":/workspace")
			data, err := os.ReadFile(filepath.Join(ws, "extension", "manifest.json")) // #nosec G304 -- the test's own workspace
			require.NoError(t, err)

			return proc.Output{}, os.WriteFile(filepath.Join(ws, "package.mcpb"), data, 0o600) // #nosec G703 -- the test's own workspace
		}

		if strings.Join(argv[:3], " ") == "docker manifest inspect" {
			return proc.Output{Code: 1}, nil
		}

		if argv[1] == "run" {
			return proc.Output{Code: exit}, nil
		}

		return proc.Output{}, nil
	}
	streamExec, toolExec, readyCheck = fake, fake, func(context.Context) bool { return true }

	t.Cleanup(func() { streamExec, toolExec, readyCheck = prevStream, prevExec, prevReady })

	return &calls
}

// make package-from-built: each target's archive and checksum land in dist, by the version.
//
//nolint:paralleltest // t.Chdir
func TestPackageTarballsThroughTheHarness(t *testing.T) {
	dir := releaseRepo(t)

	out, err := harness(t, cmdPackage, "tarballs", "--version", "v9.9.9", "--dist", "./dist", "--targets", "linux-amd64")
	require.NoError(t, err)
	assert.Equal(t, "./dist/thehivemcp-v9.9.9-linux-amd64.tar.gz (+ .sha256)\n", out)

	checksum, err := os.ReadFile(filepath.Join(dir, "dist", "thehivemcp-v9.9.9-linux-amd64.tar.gz.sha256")) // #nosec G304 -- the test's own dist
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(string(checksum), "  ./dist/thehivemcp-v9.9.9-linux-amd64.tar.gz\n"), "as shasum printed it: %s", checksum)

	var usage usageError

	_, err = harness(t, cmdPackage, "zip")
	require.ErrorAs(t, err, &usage)

	_, err = harness(t, cmdPackage, "tarballs", "--nope")
	require.ErrorAs(t, err, &usage)
}

// make mcpb-local: this machine's MCPB, with the permissions file the checkout's dotenv file names
// bundled, as the server would read it.

func TestPackageMCPBForThisMachineBundlesTheCheckoutsPermissions(t *testing.T) {
	dir := releaseRepo(t)
	gittest.Write(t, dir, map[string]string{".env": "PERMISSIONS_CONFIG=analyst.yaml\n", "analyst.yaml": "tools: {}\n"})
	t.Setenv("PERMISSIONS_CONFIG", "")

	calls := fakePacker(t, 0)

	_, err := harness(t, cmdPackage, "mcpb", "--host", "--version", "v9.9.9")
	require.NoError(t, err)
	require.Len(t, *calls, 1)

	bundle, err := os.ReadFile(filepath.Join(dir, "dist", "thehivemcp-v9.9.9-"+hostTarget+".mcpb")) // #nosec G304 -- the test's own dist
	require.NoError(t, err)
	assert.Contains(t, string(bundle), `"default": "permissions.yaml"`)
	assert.Contains(t, string(bundle), `"version": "9.9.9"`)

	// A release's bundles start read-only, whatever the packing machine has in its environment.
	t.Setenv("PERMISSIONS_CONFIG", "admin")

	_, err = harness(t, cmdPackage, "mcpb", "--version", "v9.9.9", "--targets", "linux-amd64")
	require.NoError(t, err)

	bundle, err = os.ReadFile(filepath.Join(dir, "dist", "thehivemcp-v9.9.9-linux-amd64.mcpb")) // #nosec G304 -- the test's own dist
	require.NoError(t, err)
	assert.Contains(t, string(bundle), `"default": ""`)
}

func TestSignWindowsWithoutAProviderLeavesThemUnsigned(t *testing.T) {
	dir := releaseRepo(t)
	gittest.Write(t, dir, map[string]string{"build/thehivemcp-windows-amd64.exe": "MZ"})
	t.Setenv("SIGNING_PROVIDER", "")

	out, err := harness(t, cmdSign, "build/thehivemcp-windows-amd64.exe")
	require.NoError(t, err)
	assert.Contains(t, out, "(unsigned) build/thehivemcp-windows-amd64.exe")

	_, err = harness(t, cmdSign, "build/missing.exe")
	require.ErrorContains(t, err, "binary not found")
}

// make test-integration: the stack prepared with the supplied license, then the suite in
// parallel with it; a failing suite fails the command.

func TestIntegrationPreparesTheStackThenRunsTheSuite(t *testing.T) {
	releaseRepo(t)
	t.Setenv("GO_IMAGE", "")

	_, err := harness(t, cmdIntegration)
	require.ErrorContains(t, err, "GO_IMAGE is not set")

	t.Setenv("GO_IMAGE", "golang:1.26.8")
	t.Setenv("THEHIVE_TEST_LICENSE", "eyJ.supplied")
	t.Setenv("GO_TEST_RUN", "-run TestSearch")
	t.Setenv("MOUNT_APP", "/repo:/app")

	calls := fakePacker(t, 1)

	out, err := harness(t, cmdIntegration)
	require.ErrorIs(t, err, errFailed, "the suite exited 1")
	assert.Contains(t, out, "Mode: license (parallel), supplied.")

	suite := (*calls)[len(*calls)-1]
	assert.Contains(t, suite, "THEHIVE_TEST_LICENSE=eyJ.supplied")
	assert.Contains(t, suite[len(suite)-1], "-- -run TestSearch -timeout 20m ./...", "in parallel: no -p 1")
	assert.Equal(t, "docker compose -f docker-compose.test.yml -f docker-compose.license.yml up -d", strings.Join((*calls)[0], " "))

	_, err = harness(t, cmdIntegration, "--extra")

	var usage usageError
	require.ErrorAs(t, err, &usage)
}

// make test-integration-shared: an organisation leased from devstack, no stack and no reset, the
// suite handed the lease's target and run one package at a time, and the lease given back whether
// the suite passed or failed.

// fakeDevstack stands in for docker and devstack: it records every command line, prints a lease
// and its key file's path, and makes the suite (docker run) exit with the given code.
func fakeDevstack(t *testing.T, suiteExit int) *[][]string {
	t.Helper()

	var calls [][]string

	keyFile := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(keyFile, []byte("org-admin-key\n"), 0o600))

	prevStream, prevExec, prevLook := streamExec, toolExec, lookPath
	fake := func(_ context.Context, argv []string, _ io.Reader) (proc.Output, error) {
		calls = append(calls, argv)

		switch {
		case argv[0] == "devstack" && argv[1] == "lease":
			return proc.Output{Stdout: []byte(`{"lease": "l-7", "thehive_url_from_containers": "http://thehive.devstack.orb.local:9000", "org": "thehivemcp-it", "token_file": "` + keyFile + `"}`)}, nil
		case argv[0] == "docker" && argv[1] == "run":
			return proc.Output{Code: suiteExit}, nil
		}

		return proc.Output{}, nil
	}
	streamExec, toolExec = fake, fake
	lookPath = func(string) (string, error) { return "/usr/local/bin/devstack", nil }

	t.Cleanup(func() { streamExec, toolExec, lookPath = prevStream, prevExec, prevLook })

	return &calls
}

//nolint:paralleltest // t.Chdir
func TestIntegrationSharedRunsInALeasedOrganisationAndReleasesIt(t *testing.T) {
	releaseRepo(t)
	t.Setenv("GO_IMAGE", "golang:1.26.9")
	t.Setenv("MOUNT_APP", "/repo:/app")
	t.Setenv("THEHIVE_TEST_SHARED", "1")
	t.Setenv("THEHIVE_TEST_URL", "")
	t.Setenv("THEHIVE_TEST_ORG", "")
	t.Setenv("THEHIVE_TEST_API_KEY", "")
	t.Setenv("THEHIVE_TEST_API_KEY_FILE", "")
	t.Setenv("THEHIVE_TEST_LICENSE", "eyJ.ignored")

	for _, tc := range []struct {
		name  string
		exit  int
		wantE error
	}{{"a passing suite", 0, nil}, {"a failing suite", 1, errFailed}} {
		calls := fakeDevstack(t, tc.exit)

		out, err := harness(t, cmdIntegration)
		require.ErrorIs(t, err, tc.wantE, tc.name)
		assert.Contains(t, out, "Mode: shared (sequential): organisation thehivemcp-it at http://thehive.devstack.orb.local:9000; no reset.", tc.name)

		var lines []string
		for _, c := range *calls {
			lines = append(lines, strings.Join(c, " "))
		}

		require.Len(t, lines, 3, tc.name+": lease, suite, release")
		assert.Equal(t, "devstack lease thehive --org thehivemcp-it --for TheHiveMCP integration tests", lines[0])
		assert.NotContains(t, strings.Join(lines, "\n"), "compose", "no stack, no reset")
		assert.Equal(t, "devstack release l-7", lines[2], "released after the suite, "+tc.name)

		suite := (*calls)[1]
		assert.Contains(t, suite, "THEHIVE_TEST_URL=http://thehive.devstack.orb.local:9000")
		assert.Contains(t, suite, "THEHIVE_TEST_SHARED=1")
		assert.Contains(t, suite, "THEHIVE_TEST_ORG=thehivemcp-it")
		assert.Contains(t, suite, "THEHIVE_TEST_API_KEY=org-admin-key")
		assert.Contains(t, suite, "THEHIVE_TEST_LICENSE=", "no license in the shared mode")
		assert.Contains(t, suite[len(suite)-1], "-p 1 -timeout 20m ./...", "one package at a time")
	}
}

// Without devstack and without a key the run stops before anything is started.
//
//nolint:paralleltest // t.Chdir
func TestIntegrationSharedWithoutATargetStopsEarly(t *testing.T) {
	releaseRepo(t)
	t.Setenv("GO_IMAGE", "golang:1.26.9")
	t.Setenv("THEHIVE_TEST_SHARED", "true")
	t.Setenv("THEHIVE_TEST_API_KEY", "")
	t.Setenv("THEHIVE_TEST_API_KEY_FILE", "")

	calls := fakeDevstack(t, 0)
	lookPath = func(string) (string, error) { return "", os.ErrNotExist }

	_, err := harness(t, cmdIntegration)
	require.ErrorContains(t, err, "THEHIVE_TEST_API_KEY_FILE")
	assert.Empty(t, *calls, "no stack, no suite")
}

// A false THEHIVE_TEST_SHARED is the default mode: the stack comes up, nothing is leased.
//
//nolint:paralleltest // t.Chdir
func TestIntegrationSharedOffKeepsTheDefaultMode(t *testing.T) {
	releaseRepo(t)
	t.Setenv("GO_IMAGE", "golang:1.26.9")
	t.Setenv("THEHIVE_TEST_SHARED", "0")
	t.Setenv("THEHIVE_TEST_LICENSE", "eyJ.supplied")

	calls := fakeDevstack(t, 0)

	out, err := harness(t, cmdIntegration)
	require.NoError(t, err)
	assert.Contains(t, out, "Mode: license (parallel), supplied.")
	assert.Equal(t, "docker compose -f docker-compose.test.yml -f docker-compose.license.yml up -d", strings.Join((*calls)[0], " "))

	for _, c := range *calls {
		assert.NotEqual(t, "devstack", c[0])
	}
}

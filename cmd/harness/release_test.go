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
	gittest.Write(t, dir, map[string]string{"build/thehivemcp-linux-amd64": "elf", "build/thehivemcp-" + hostTarget: "elf"})
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

	out, err := harness(t, cmdPackage, "tarballs", "--version", "v9.9.9", "--targets", "linux-amd64")
	require.NoError(t, err)
	assert.Contains(t, out, "thehivemcp-v9.9.9-linux-amd64.tar.gz (+ .sha256)")
	assert.FileExists(t, filepath.Join(dir, "dist", "thehivemcp-v9.9.9-linux-amd64.tar.gz.sha256"))

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

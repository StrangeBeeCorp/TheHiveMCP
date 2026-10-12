package release_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/proc"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/release"
)

const (
	bundled  = "permissions.yaml"
	readOnly = "read_only"
)

// The manifest names the binary as the target needs it, carries the version without its v, and
// the permissions default; the API key is marked sensitive.
func TestManifestDescribesTheBundle(t *testing.T) {
	t.Parallel()

	data, err := release.Manifest("1.2.0", ".exe", bundled)
	require.NoError(t, err)

	var m struct {
		Version string `json:"version"`
		Server  struct {
			EntryPoint string `json:"entry_point"`
			MCPConfig  struct {
				Command string            `json:"command"`
				Args    []string          `json:"args"`
				Env     map[string]string `json:"env"`
			} `json:"mcp_config"`
		} `json:"server"`
		UserConfig map[string]struct {
			Required  bool    `json:"required"`
			Sensitive bool    `json:"sensitive"`
			Default   *string `json:"default"`
		} `json:"user_config"`
	}
	require.NoError(t, json.Unmarshal(data, &m))

	assert.Equal(t, "1.2.0", m.Version)
	assert.Equal(t, "server/thehivemcp.exe", m.Server.EntryPoint)
	assert.Equal(t, "${__dirname}/server/thehivemcp.exe", m.Server.MCPConfig.Command)
	assert.Equal(t, []string{"-transport", "stdio"}, m.Server.MCPConfig.Args)
	assert.Equal(t, "${user_config.permissions_config}", m.Server.MCPConfig.Env["PERMISSIONS_CONFIG"])
	assert.True(t, m.UserConfig["thehive_api_key"].Sensitive)
	assert.True(t, m.UserConfig["thehive_url"].Required)
	assert.False(t, m.UserConfig["organisation"].Required)
	require.NotNil(t, m.UserConfig["permissions_config"].Default)
	assert.Equal(t, bundled, *m.UserConfig["permissions_config"].Default)

	empty, err := release.Manifest("1.2.0", "", "")
	require.NoError(t, err)
	assert.Contains(t, string(empty), `"default": ""`, "the default is said, empty, as before")
	assert.Contains(t, string(empty), `"entry_point": "server/thehivemcp"`)
}

// The manifest is the one generate-mcpb.sh wrote, compared as JSON: testdata holds its Windows
// rendering with a bundled permissions file, and the Linux one differs by the .exe and the default.
func TestManifestIsTheOneTheScriptWrote(t *testing.T) {
	t.Parallel()

	golden, err := os.ReadFile(filepath.Join("testdata", "manifest-windows.json"))
	require.NoError(t, err)

	windows, err := release.Manifest("1.2.0", ".exe", bundled)
	require.NoError(t, err)
	assert.JSONEq(t, string(golden), string(windows))

	linux, err := release.Manifest("1.2.0", "", "")
	require.NoError(t, err)

	want := strings.ReplaceAll(string(golden), "thehivemcp.exe", "thehivemcp")
	want = strings.Replace(want, `"default": "permissions.yaml"`, `"default": ""`, 1)
	assert.JSONEq(t, want, string(linux))
}

// A path to a file, absolute or relative to the checkout, is bundled; admin, read_only, nothing,
// or a path that names no file here is the default as it is.
func TestPermissionsFromReadsTheServersSetting(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	file := filepath.Join(root, "analyst.yaml")
	require.NoError(t, os.WriteFile(file, []byte("tools: {}\n"), 0o600))

	assert.Equal(t, release.Permissions{File: file, Default: bundled}, release.PermissionsFrom(file, "/elsewhere"))
	assert.Equal(t, release.Permissions{File: file, Default: bundled}, release.PermissionsFrom("analyst.yaml", root))
	assert.Equal(t, release.Permissions{Default: readOnly}, release.PermissionsFrom(readOnly, root))
	assert.Equal(t, release.Permissions{Default: "admin"}, release.PermissionsFrom("admin", root))
	assert.Equal(t, release.Permissions{}, release.PermissionsFrom("", root))
	assert.Equal(t, release.Permissions{Default: "config/analyst.yaml"}, release.PermissionsFrom("config/analyst.yaml", root),
		"a path of the client machine stays as it was given")
}

// The extension holds the manifest, the icon, the server binary (executable outside Windows) and
// the bundled permissions.
func TestExtensionLaysOutTheBundle(t *testing.T) {
	t.Parallel()

	o := built(t)
	perms := filepath.Join(t.TempDir(), "analyst.yaml")
	require.NoError(t, os.WriteFile(perms, []byte("tools: {}\n"), 0o600))

	icon := filepath.Join(t.TempDir(), "logo.png")
	require.NoError(t, os.WriteFile(icon, []byte("png"), 0o600))

	dir := filepath.Join(t.TempDir(), "extension")
	x := release.Extension{Version: "1.2.0", Binary: filepath.Join(o.Build, linuxBinary), Icon: icon, Permissions: release.PermissionsFrom(perms, "")}
	require.NoError(t, x.Write(dir))

	info, err := os.Stat(filepath.Join(dir, "server", "thehivemcp"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())

	for file, want := range map[string]string{"icon.png": "png", bundled: "tools: {}\n", "server/thehivemcp": elf} {
		got, err := os.ReadFile(filepath.Join(dir, file)) // #nosec G304 -- the test's own extension
		require.NoError(t, err, file)
		assert.Equal(t, want, string(got), file)
	}

	manifest, err := os.ReadFile(filepath.Join(dir, "manifest.json")) // #nosec G304 -- the test's own extension
	require.NoError(t, err)
	assert.Contains(t, string(manifest), `"default": "permissions.yaml"`)
}

// Each target is laid out on a workspace of its own and packed by the packer image as the host
// user; the bundle lands in dist by its target, with its .sha256.
func TestMCPBsPackEachTarget(t *testing.T) {
	t.Parallel()

	o := built(t)
	icon := filepath.Join(t.TempDir(), "logo.png")
	require.NoError(t, os.WriteFile(icon, []byte("png"), 0o600))

	var calls [][]string

	pack := func(_ context.Context, argv []string, _ io.Reader) (proc.Output, error) {
		calls = append(calls, argv)
		ws := strings.TrimSuffix(argv[6], ":/workspace")

		manifest, err := os.ReadFile(filepath.Join(ws, "extension", "manifest.json")) // #nosec G304 -- the test's own workspace
		require.NoError(t, err)

		return proc.Output{}, os.WriteFile(filepath.Join(ws, "package.mcpb"), manifest, 0o600) // #nosec G703 -- the test's own workspace
	}

	written, err := release.MCPBs(t.Context(), pack, o, icon, release.Permissions{Default: readOnly}, 1000, 1001)
	require.NoError(t, err)
	assert.Equal(t, []string{"thehivemcp-v1.2.0-linux-amd64.mcpb", "thehivemcp-v1.2.0-windows-arm64.mcpb"}, written)

	require.Len(t, calls, 2)
	assert.Equal(t, []string{"docker", "run", "--rm", "--user", "1000:1001"}, calls[0][:5])
	assert.Equal(t, []string{"-e", "HOME=/workspace", release.MCPBImage, "pack", "/workspace/extension", "/workspace/package.mcpb"}, calls[0][7:])

	win, err := os.ReadFile(filepath.Join(o.Dist, written[1])) // #nosec G304 -- the test's own dist
	require.NoError(t, err)
	assert.Contains(t, string(win), `"entry_point": "server/thehivemcp.exe"`)
	assert.Contains(t, string(win), `"version": "1.2.0"`)
	assert.FileExists(t, filepath.Join(o.Dist, written[1]+".sha256"))
}

// A packer that fails stops the run with what it printed.
func TestMCPBsReportAFailedPack(t *testing.T) {
	t.Parallel()

	o := built(t)
	fail := func(context.Context, []string, io.Reader) (proc.Output, error) {
		return proc.Output{Stderr: []byte("manifest invalid: version"), Code: 1}, nil
	}

	_, err := release.MCPBs(t.Context(), fail, o, filepath.Join(o.Build, linuxBinary), release.Permissions{}, 1, 1)
	require.ErrorContains(t, err, "package the linux-amd64 MCPB: mcpb pack exited 1: manifest invalid: version")

	o.Targets = []string{"darwin-arm64"}
	_, err = release.MCPBs(t.Context(), fail, o, filepath.Join(o.Build, linuxBinary), release.Permissions{}, 1, 1)
	require.ErrorContains(t, err, "thehivemcp-darwin-arm64", "a missing binary is said before the packer runs")
}

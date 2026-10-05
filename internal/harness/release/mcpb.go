package release

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/proc"
)

// MCPBImage is the packaging image make mcpb-build-image builds from scripts/Dockerfile.mcpb:
// node with the pinned @anthropic-ai/mcpb packer as its entrypoint.
const MCPBImage = "thehivemcp-mcpb:latest"

// Permissions is the permissions configuration an MCPB carries: a file bundled as
// permissions.yaml, or, when it names no file, the default the user's client starts from ("",
// admin, read_only).
type Permissions struct {
	File    string
	Default string
}

// PermissionsFrom reads PERMISSIONS_CONFIG as the server does: a path to an existing file is
// bundled, any other value is the default as it is.
func PermissionsFrom(value string) Permissions {
	if value != "" && value != "admin" && value != "read_only" {
		info, err := os.Stat(value)
		if err == nil && info.Mode().IsRegular() {
			return Permissions{File: value, Default: bundledPermissions}
		}
	}

	return Permissions{Default: value}
}

const bundledPermissions = "permissions.yaml"

// Extension is one MCPB's directory before packing.
type Extension struct {
	// Version is the manifest's, without the leading v.
	Version string
	// Binary is the built binary shipped as server/thehivemcp, with Ext (.exe on Windows).
	Binary string
	Ext    string
	// Icon is the PNG shown by the client.
	Icon        string
	Permissions Permissions
}

// Write lays the extension out in dir: manifest.json, icon.png, server/thehivemcp[.exe], and
// permissions.yaml when a file is bundled.
func (x Extension) Write(dir string) error {
	err := os.MkdirAll(filepath.Join(dir, "server"), dirPerm)
	if err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	copies := []struct {
		from, to string
		mode     os.FileMode
	}{
		{x.Binary, "server/thehivemcp" + x.Ext, execPerm}, // the .exe too, as the build leaves it
		{x.Icon, "icon.png", filePerm},
	}
	if x.Permissions.File != "" {
		copies = append(copies, struct {
			from, to string
			mode     os.FileMode
		}{x.Permissions.File, bundledPermissions, filePerm})
	}

	for _, c := range copies {
		err = copyFile(c.from, filepath.Join(dir, c.to), c.mode)
		if err != nil {
			return err
		}
	}

	manifest, err := Manifest(x.Version, x.Ext, x.Permissions.Default)
	if err != nil {
		return err
	}

	return writeAtomic(filepath.Join(dir, "manifest.json"), func(w io.Writer) error {
		_, err := w.Write(manifest)
		return err //nolint:wrapcheck // Write names the extension
	})
}

// MCPBs packs, for each target, its binary into <name>-<version>-<target>.mcpb in o.Dist, with
// a .sha256 beside it, by running the packer image on a workspace of its own. uid and gid are
// the host user's: the packer writes into the mounted workspace as them.
func MCPBs(ctx context.Context, run proc.Exec, o Options, icon string, perms Permissions, uid, gid int) ([]string, error) {
	if len(o.Targets) == 0 {
		return nil, ErrNoTarget
	}

	err := os.MkdirAll(o.Dist, dirPerm)
	if err != nil {
		return nil, fmt.Errorf("create %s: %w", o.Dist, err)
	}

	var written []string

	for _, target := range o.Targets {
		name, err := mcpb(ctx, run, o, target, icon, perms, uid, gid)
		if err != nil {
			return written, fmt.Errorf("package the %s MCPB: %w", target, err)
		}

		written = append(written, name)
	}

	return written, nil
}

func mcpb(ctx context.Context, run proc.Exec, o Options, target, icon string, perms Permissions, uid, gid int) (string, error) {
	ws, err := os.MkdirTemp("", "mcpb-"+target+"-")
	if err != nil {
		return "", fmt.Errorf("create a workspace: %w", err)
	}
	defer func() { _ = os.RemoveAll(ws) }()

	ext := ""
	if strings.HasPrefix(target, "windows-") {
		ext = ".exe"
	}

	x := Extension{
		Version: strings.TrimPrefix(o.Version, "v"), Binary: filepath.Join(o.Build, Binary(o.Name, target)), Ext: ext,
		Icon: icon, Permissions: perms,
	}

	err = x.Write(filepath.Join(ws, "extension"))
	if err != nil {
		return "", err
	}

	out, err := run(ctx, PackArgv(ws, uid, gid), nil)
	if err == nil && out.Code != 0 {
		err = fmt.Errorf("mcpb pack exited %d: %s", out.Code, strings.TrimSpace(out.Text()))
	}

	if err != nil {
		return "", err
	}

	name := o.Name + "-" + o.Version + "-" + target + ".mcpb"

	err = copyFile(filepath.Join(ws, packed), filepath.Join(o.Dist, name), filePerm)
	if err == nil {
		err = WriteChecksum(o.Dist, name)
	}

	return name, err
}

const packed = "package.mcpb"

// PackArgv runs the packer on workspace/extension, writing workspace/package.mcpb as the host
// user, whose home is the workspace (the image has none for that user).
func PackArgv(workspace string, uid, gid int) []string {
	return []string{
		"docker", "run", "--rm", "--user", fmt.Sprintf("%d:%d", uid, gid), "-v", workspace + ":/workspace", "-e", "HOME=/workspace",
		MCPBImage, "pack", "/workspace/extension", "/workspace/" + packed,
	}
}

func copyFile(from, to string, mode os.FileMode) error {
	data, err := os.ReadFile(from) // #nosec G304 -- the binary, icon or permissions file a release packs
	if err != nil {
		return fmt.Errorf("read %s: %w", from, err)
	}

	err = os.WriteFile(to, data, mode) // #nosec G306 G703 -- a workspace file of this run, packed into a public release artifact
	if err == nil {
		err = os.Chmod(to, mode) // WriteFile's mode is masked by the umask
	}

	if err != nil {
		return fmt.Errorf("write %s: %w", to, err)
	}

	return nil
}

// jsonString is the type of every user_config field.
const jsonString = "string"

// Manifest is the MCPB manifest.json of a version, a binary extension (.exe on Windows) and the
// permissions default the user's client starts from.
func Manifest(version, ext, permissionsDefault string) ([]byte, error) {
	type field struct {
		Title       string  `json:"title"`
		Description string  `json:"description"`
		Type        string  `json:"type"`
		Required    bool    `json:"required"`
		Sensitive   bool    `json:"sensitive,omitempty"`
		Default     *string `json:"default,omitempty"`
	}

	command := "${__dirname}/server/thehivemcp" + ext
	m := struct {
		ManifestVersion string `json:"manifest_version"`
		Name            string `json:"name"`
		Version         string `json:"version"`
		Description     string `json:"description"`
		LongDescription string `json:"long_description"`
		Author          struct {
			Name  string `json:"name"`
			Email string `json:"email"`
			URL   string `json:"url"`
		} `json:"author"`
		Homepage string `json:"homepage"`
		Icon     string `json:"icon"`
		Server   struct {
			Type       string `json:"type"`
			EntryPoint string `json:"entry_point"`
			MCPConfig  struct {
				Command string            `json:"command"`
				Args    []string          `json:"args"`
				Env     map[string]string `json:"env"`
			} `json:"mcp_config"`
		} `json:"server"`
		UserConfig struct {
			TheHiveURL        field `json:"thehive_url"`
			TheHiveAPIKey     field `json:"thehive_api_key"`
			Organisation      field `json:"organisation"`
			PermissionsConfig field `json:"permissions_config"`
		} `json:"user_config"`
	}{
		ManifestVersion: "0.2", Name: "TheHiveMCP", Version: version,
		Description:     "Connect to TheHive and Cortex via Hivemind MCP for security incident management",
		LongDescription: "StrangeBee Hivemind MCP enables Claude to interact with TheHive security incident response platform and Cortex analysis engine...",
		Homepage:        "https://docs.strangebee.com/mcp", Icon: "icon.png",
	}

	m.Author.Name, m.Author.Email, m.Author.URL = "StrangeBee", "admin@strangebee.com", "https://strangebee.com"
	m.Server.Type, m.Server.EntryPoint = "binary", "server/thehivemcp"+ext
	m.Server.MCPConfig.Command, m.Server.MCPConfig.Args = command, []string{"-transport", "stdio"}
	m.Server.MCPConfig.Env = map[string]string{
		"MCP_PORT": "8082", "THEHIVE_URL": "${user_config.thehive_url}", "THEHIVE_API_KEY": "${user_config.thehive_api_key}",
		"THEHIVE_ORGANISATION": "${user_config.organisation}", "PERMISSIONS_CONFIG": "${user_config.permissions_config}",
	}
	m.UserConfig.TheHiveURL = field{Title: "TheHive URL", Description: "Your TheHive instance URL (e.g., https://thehive.company.com)", Type: jsonString, Required: true}
	m.UserConfig.TheHiveAPIKey = field{Title: "TheHive API Key", Description: "Your TheHive API key for authentication", Type: jsonString, Required: true, Sensitive: true}
	m.UserConfig.Organisation = field{
		Title: "Organisation", Description: "TheHive organisation name. If not set, defaults to the authenticated user's own organisation.", Type: jsonString,
	}
	m.UserConfig.PermissionsConfig = field{
		Title: "Permissions Config", Type: jsonString, Default: &permissionsDefault,
		Description: "Path to permissions YAML config file (leave empty for read-only default, or use bundled relative path)",
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}

	return append(data, '\n'), nil
}

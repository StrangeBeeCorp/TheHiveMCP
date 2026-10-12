package release

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/proc"
)

// Signer signs the Windows binaries in place (ADR 0002). Its provider is SIGNING_PROVIDER:
//   - none, the default: the binaries stay unsigned (local builds, and the interim unsigned
//     release of DL-6329);
//   - signpath: the SignPath action of release.yml signs them, out of band; the run checks that
//     they exist;
//   - azure: Azure Artifact Signing, through the sign tool (dotnet sign), installed when missing.
type Signer struct {
	Provider string
	// Env reads the AZURE_SIGNING_* settings; Look finds a tool on PATH; Home is the user's
	// home, where dotnet installs its global tools.
	Env  func(string) string
	Look func(string) (string, error)
	Home string
	Run  proc.Exec
	Out  io.Writer
}

// The sign tool's version, and the timestamp authority Azure Artifact Signing signs against.
const (
	signVersion  = "0.9.1-*"
	timestampURL = "http://timestamp.acs.microsoft.com" // RFC 3161 timestamping is http by design: the token is signed
)

// Sign checks that every binary exists, then signs them with the provider.
func (s Signer) Sign(ctx context.Context, binaries []string) error {
	if len(binaries) == 0 {
		_, _ = fmt.Fprintln(s.Out, "sign-windows: no binaries passed; nothing to sign.")
		return nil
	}

	for _, b := range binaries {
		info, err := os.Stat(b)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("sign-windows: binary not found: %s", b)
		}
	}

	switch provider := s.provider(); provider {
	case "none":
		_, _ = fmt.Fprintln(s.Out, "sign-windows: SIGNING_PROVIDER=none: the binaries stay UNSIGNED (interim, DL-6329; ADR 0002):")
		for _, b := range binaries {
			_, _ = fmt.Fprintln(s.Out, "  (unsigned)", b)
		}
	case "signpath":
		_, _ = fmt.Fprintln(s.Out, "sign-windows: SIGNING_PROVIDER=signpath: the SignPath action of release.yml signs them. They exist:")
		for _, b := range binaries {
			_, _ = fmt.Fprintln(s.Out, " ", b)
		}
	case "azure":
		return s.azure(ctx, binaries)
	default:
		return fmt.Errorf("sign-windows: unknown SIGNING_PROVIDER %q (expected signpath, azure or none)", provider)
	}

	return nil
}

func (s Signer) provider() string {
	if s.Provider == "" {
		return "none"
	}

	return s.Provider
}

func (s Signer) azure(ctx context.Context, binaries []string) error {
	settings := map[string]string{}

	var missing []string

	for _, k := range []string{"AZURE_SIGNING_ENDPOINT", "AZURE_SIGNING_ACCOUNT", "AZURE_SIGNING_PROFILE"} {
		settings[k] = s.Env(k)
		if settings[k] == "" {
			missing = append(missing, k)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("sign-windows: set %s", strings.Join(missing, ", "))
	}

	tool, err := s.signTool(ctx)
	if err != nil {
		return err
	}

	for _, b := range binaries {
		_, _ = fmt.Fprintf(s.Out, "sign-windows: signing %s with Azure Artifact Signing (%s)...\n", b, settings["AZURE_SIGNING_PROFILE"])

		err = s.check(s.Run(ctx, []string{
			tool, "code", "trusted-signing", b,
			"--trusted-signing-endpoint", settings["AZURE_SIGNING_ENDPOINT"],
			"--trusted-signing-account", settings["AZURE_SIGNING_ACCOUNT"],
			"--trusted-signing-certificate-profile", settings["AZURE_SIGNING_PROFILE"],
			"--timestamp-url", timestampURL, "--file-digest", "SHA256",
		}, nil))
		if err != nil {
			return fmt.Errorf("sign-windows: sign %s: %w", b, err)
		}
	}

	_, _ = fmt.Fprintln(s.Out, "sign-windows: Azure signing complete.")

	return nil
}

// signTool is the sign tool on PATH, or the one dotnet installs as a global tool.
func (s Signer) signTool(ctx context.Context) (string, error) {
	path, err := s.Look("sign")
	if err == nil {
		return path, nil
	}

	_, _ = fmt.Fprintln(s.Out, "sign-windows: installing the sign tool (dotnet sign)...")

	err = s.check(s.Run(ctx, []string{"dotnet", "tool", "install", "--global", "sign", "--version", signVersion}, nil))
	if err != nil {
		return "", fmt.Errorf("sign-windows: install the sign tool: %w", err)
	}

	return filepath.Join(s.Home, ".dotnet", "tools", "sign"), nil
}

func (s Signer) check(out proc.Output, err error) error {
	if err != nil {
		return err
	}

	if out.Code == 0 {
		return nil
	}

	if text := strings.TrimSpace(out.Text()); text != "" {
		return errors.New(text)
	}

	return fmt.Errorf("exited %d", out.Code) // its output streamed to the log
}

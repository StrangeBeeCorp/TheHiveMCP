package release_test

import (
	"context"
	"errors"
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
	azure   = "azure"
	account = "strangebee"
)

var azureSettings = map[string]string{
	"AZURE_SIGNING_ENDPOINT": "https://weu.codesigning.azure.net", "AZURE_SIGNING_ACCOUNT": account, "AZURE_SIGNING_PROFILE": "PublicTrust",
}

type signer struct {
	release.Signer

	calls [][]string
	out   strings.Builder
}

func newSigner(provider string, onPath bool, code int) *signer {
	s := &signer{}
	s.Signer = release.Signer{
		Provider: provider, Env: func(k string) string { return azureSettings[k] }, Home: "/home/runner",
		Look: func(string) (string, error) {
			if onPath {
				return "/usr/local/bin/sign", nil
			}

			return "", errors.New("not found")
		},
		Run: func(_ context.Context, argv []string, _ io.Reader) (proc.Output, error) {
			s.calls = append(s.calls, argv)
			return proc.Output{Code: code}, nil
		},
		Out: &s.out,
	}

	return s
}

func exe(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), winBinary)
	require.NoError(t, os.WriteFile(path, []byte(pe), 0o600))

	return path
}

// Without a provider the binaries stay unsigned, and the run says so; with signpath the
// workflow's action signs them, and the run only checks that they exist.
func TestSignLeavesTheSigningOutOfBandWithoutAzure(t *testing.T) {
	t.Parallel()

	bin := exe(t)

	for _, provider := range []string{"", "none", "signpath"} {
		s := newSigner(provider, true, 0)
		require.NoError(t, s.Sign(t.Context(), []string{bin}), provider)
		assert.Empty(t, s.calls, "%q runs no tool", provider)
		assert.Contains(t, s.out.String(), bin)
	}

	assert.Contains(t, newSignerOut(t, "", bin), "UNSIGNED")

	require.ErrorContains(t, newSigner("none", true, 0).Sign(t.Context(), []string{bin, "/missing.exe"}), "binary not found: /missing.exe",
		"every binary is checked before anything runs")
	require.ErrorContains(t, newSigner("gpg", true, 0).Sign(t.Context(), []string{bin}), `unknown SIGNING_PROVIDER "gpg"`)
	require.NoError(t, newSigner(azure, true, 0).Sign(t.Context(), nil), "no binary, nothing to sign")
}

func newSignerOut(t *testing.T, provider, bin string) string {
	t.Helper()

	s := newSigner(provider, true, 0)
	require.NoError(t, s.Sign(t.Context(), []string{bin}))

	return s.out.String()
}

// With azure each binary is signed by the sign tool, installed by dotnet when it is missing.
func TestSignWithAzureArtifactSigning(t *testing.T) {
	t.Parallel()

	bin := exe(t)

	s := newSigner(azure, true, 0)
	require.NoError(t, s.Sign(t.Context(), []string{bin}))
	assert.Equal(t, [][]string{{
		"/usr/local/bin/sign", "code", "trusted-signing", bin,
		"--trusted-signing-endpoint", "https://weu.codesigning.azure.net", "--trusted-signing-account", account,
		"--trusted-signing-certificate-profile", "PublicTrust", "--timestamp-url", "http://timestamp.acs.microsoft.com", "--file-digest", "SHA256",
	}}, s.calls)

	s = newSigner(azure, false, 0)
	require.NoError(t, s.Sign(t.Context(), []string{bin}))
	require.Len(t, s.calls, 2)
	assert.Equal(t, []string{"dotnet", "tool", "install", "--global", "sign", "--version", "0.9.1-*"}, s.calls[0])
	assert.Equal(t, "/home/runner/.dotnet/tools/sign", s.calls[1][0])

	require.ErrorContains(t, newSigner(azure, true, 1).Sign(t.Context(), []string{bin}), "sign "+bin+": exited 1")
}

func TestSignWithAzureNeedsItsSettings(t *testing.T) {
	t.Parallel()

	s := newSigner(azure, true, 0)
	s.Env = func(k string) string {
		if k == "AZURE_SIGNING_ACCOUNT" {
			return account
		}

		return ""
	}

	require.ErrorContains(t, s.Sign(t.Context(), []string{exe(t)}), "set AZURE_SIGNING_ENDPOINT, AZURE_SIGNING_PROFILE")
	assert.Empty(t, s.calls)
}

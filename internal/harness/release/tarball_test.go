package release_test

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/release"
)

const (
	name        = "thehivemcp"
	version     = "v1.2.0"
	linux       = "linux-amd64"
	windows     = "windows-arm64"
	linuxBinary = "thehivemcp-linux-amd64"
	winBinary   = "thehivemcp-windows-arm64.exe"
	elf         = "\x7fELF the server"
	pe          = "MZ the server"
)

// built is a build directory holding a Linux and a Windows binary, and an empty dist beside it.
func built(t *testing.T) release.Options {
	t.Helper()

	dir := t.TempDir()
	build := filepath.Join(dir, "build")
	require.NoError(t, os.MkdirAll(build, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(build, linuxBinary), []byte(elf), 0o755)) // #nosec G306 -- a test binary
	require.NoError(t, os.WriteFile(filepath.Join(build, winBinary), []byte(pe), 0o644))    // #nosec G306 -- a test binary

	return release.Options{Version: version, Build: build, Dist: filepath.Join(dir, "dist"), Name: name, Targets: []string{linux, windows}}
}

func TestBinaryNamesWindowsWithExe(t *testing.T) {
	t.Parallel()

	assert.Equal(t, linuxBinary, release.Binary(name, linux))
	assert.Equal(t, winBinary, release.Binary(name, windows))
}

// Each target's binary goes alone into its archive, by its name and with its mode, and a .sha256
// beside it says the archive's hash as shasum -a 256 prints it.
func TestTarballsPackEachBinaryWithItsChecksum(t *testing.T) {
	t.Parallel()

	o := built(t)
	o.DistAs = "./dist"

	written, err := release.Tarballs(o)
	require.NoError(t, err)
	assert.Equal(t, []string{"thehivemcp-v1.2.0-linux-amd64.tar.gz", "thehivemcp-v1.2.0-windows-arm64.tar.gz"}, written)

	name, mode, content := untar(t, filepath.Join(o.Dist, written[0]))
	assert.Equal(t, linuxBinary, name)
	assert.Equal(t, int64(0o755), mode, "the binary stays executable")
	assert.Equal(t, elf, content)

	name, _, content = untar(t, filepath.Join(o.Dist, written[1]))
	assert.Equal(t, winBinary, name)
	assert.Equal(t, pe, content)

	archive, err := os.ReadFile(filepath.Join(o.Dist, written[0]))
	require.NoError(t, err)

	sum := sha256.Sum256(archive)
	checksum, err := os.ReadFile(filepath.Join(o.Dist, written[0]+".sha256"))
	require.NoError(t, err)
	assert.Equal(t, hex.EncodeToString(sum[:])+"  ./dist/"+written[0]+"\n", string(checksum), "dist as the caller named it: no machine's path")
}

// A binary that is missing stops the run on its target, and leaves no half-written archive.
func TestTarballsStopOnAMissingBinary(t *testing.T) {
	t.Parallel()

	o := built(t)
	o.Targets = []string{linux, "darwin-arm64"}

	written, err := release.Tarballs(o)
	require.ErrorContains(t, err, "package darwin-arm64")
	assert.Len(t, written, 1)

	entries, err := os.ReadDir(o.Dist)
	require.NoError(t, err)
	assert.Len(t, entries, 2, "the linux archive and its checksum, nothing of darwin")

	_, err = release.Tarballs(release.Options{Dist: t.TempDir()})
	require.ErrorIs(t, err, release.ErrNoTarget)
}

// An archive that cannot be put in place leaves neither it nor its temporary file in dist.
func TestTarballsLeaveNoTemporaryFileBehind(t *testing.T) {
	t.Parallel()

	o := built(t)
	o.Targets = []string{linux}
	blocked := filepath.Join(o.Dist, "thehivemcp-v1.2.0-linux-amd64.tar.gz")
	require.NoError(t, os.MkdirAll(filepath.Join(blocked, "in-the-way"), 0o750), "a directory where the archive goes")

	_, err := release.Tarballs(o)
	require.ErrorContains(t, err, "package linux-amd64")

	entries, err := os.ReadDir(o.Dist)
	require.NoError(t, err)
	require.Len(t, entries, 1, "the temporary file is removed")
	assert.True(t, entries[0].IsDir())
}

func untar(t *testing.T, path string) (string, int64, string) {
	t.Helper()

	f, err := os.Open(path) // #nosec G304 -- the test's own archive
	require.NoError(t, err)

	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	require.NoError(t, err)

	tr := tar.NewReader(gz)
	header, err := tr.Next()
	require.NoError(t, err)

	content, err := io.ReadAll(tr)
	require.NoError(t, err)

	_, err = tr.Next()
	require.ErrorIs(t, err, io.EOF, "one file per archive")

	return header.Name, header.Mode, string(content)
}

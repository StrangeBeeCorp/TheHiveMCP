// Package release packages the built binaries for a release: a .tar.gz and an MCPB bundle per
// target, each with its .sha256, and the Windows binaries signed. bin/harness package and
// bin/harness sign-windows run it for make package-from-built and the release workflow.
package release

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Options name a release's binaries and where its packages go.
type Options struct {
	// Version is baked into the package names: v1.2.0, or a snapshot name.
	Version string
	// Build holds the built binaries, <name>-<target>[.exe]; Dist receives the packages.
	Build string
	Dist  string
	// DistAs is how the checksum files name Dist: as the caller gave it (./dist), so that a
	// published .sha256 carries no machine's path. Dist itself when empty.
	DistAs string
	// Name is the binary's base name.
	Name    string
	Targets []string
}

const (
	dirPerm  = 0o750
	filePerm = 0o644
	execPerm = 0o755
)

// ErrNoTarget is a run given no target to package.
var ErrNoTarget = errors.New("no target to package")

func (o Options) distAs() string {
	if o.DistAs != "" {
		return o.DistAs
	}

	return o.Dist
}

// Binary is a target's binary name: Windows binaries end in .exe.
func Binary(name, target string) string {
	if strings.HasPrefix(target, "windows-") {
		return name + "-" + target + ".exe"
	}

	return name + "-" + target
}

// Tarballs packs each target's binary alone into <name>-<version>-<target>.tar.gz, with a
// .sha256 beside it, and returns the archives written.
func Tarballs(o Options) ([]string, error) {
	if len(o.Targets) == 0 {
		return nil, ErrNoTarget
	}

	err := os.MkdirAll(o.Dist, dirPerm)
	if err != nil {
		return nil, fmt.Errorf("create %s: %w", o.Dist, err)
	}

	var written []string

	for _, target := range o.Targets {
		archive := o.Name + "-" + o.Version + "-" + target + ".tar.gz"

		err = tarball(filepath.Join(o.Build, Binary(o.Name, target)), filepath.Join(o.Dist, archive))
		if err == nil {
			err = WriteChecksum(o.Dist, o.distAs(), archive)
		}

		if err != nil {
			return written, fmt.Errorf("package %s: %w", target, err)
		}

		written = append(written, archive)
	}

	return written, nil
}

// tarball writes a gzipped tar of the one file binary, by its base name, to out.
func tarball(binary, out string) (err error) {
	in, err := os.Open(binary) // #nosec G304 -- a binary the release built
	if err != nil {
		return err //nolint:wrapcheck // Tarballs names the target
	}
	defer func() { _ = in.Close() }()

	info, err := in.Stat()
	if err != nil {
		return err //nolint:wrapcheck // Tarballs names the target
	}

	return writeAtomic(out, filePerm, func(w io.Writer) error {
		gz := gzip.NewWriter(w)
		tw := tar.NewWriter(gz)

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err //nolint:wrapcheck // Tarballs names the target
		}

		header.Name = filepath.Base(binary)

		err = tw.WriteHeader(header)
		if err == nil {
			_, err = io.Copy(tw, in)
		}

		return errors.Join(err, tw.Close(), gz.Close())
	})
}

// WriteChecksum writes dist/file's SHA-256 to dist/file.sha256 as shasum -a 256 prints it: the
// hash, two spaces and the path, as/file, where as names dist as the caller gave it.
func WriteChecksum(dist, as, file string) error {
	path := as + "/" + file

	data, err := os.ReadFile(filepath.Join(dist, file)) // #nosec G304 -- a package this run wrote
	if err != nil {
		return err //nolint:wrapcheck // the caller names the package
	}

	sum := sha256.Sum256(data)
	line := hex.EncodeToString(sum[:]) + "  " + path + "\n"

	return writeAtomic(filepath.Join(dist, file+".sha256"), filePerm, func(w io.Writer) error {
		_, err := io.WriteString(w, line)
		return err //nolint:wrapcheck // the caller names the package
	})
}

// writeAtomic writes path, with mode, through a temporary file renamed into place, so that a
// failed run leaves no half-written package.
func writeAtomic(path string, mode os.FileMode, write func(io.Writer) error) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err //nolint:wrapcheck // the caller names the package
	}

	err = errors.Join(write(tmp), tmp.Close())
	if err == nil {
		err = os.Chmod(tmp.Name(), mode)
	}

	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}

	if err != nil {
		_ = os.Remove(tmp.Name())
	}

	return err //nolint:wrapcheck // the caller names the package
}

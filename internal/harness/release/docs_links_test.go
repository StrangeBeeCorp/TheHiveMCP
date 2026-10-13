package release_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/release"
)

const repoRoot = "../../.."

var (
	releaseURL = regexp.MustCompile("https://github\\.com/StrangeBeeCorp/TheHiveMCP/releases/([^\\s\"'<>)`]*download/[^\\s\"'<>)`]+)")
	docAsset   = regexp.MustCompile(`^download/\$\{VERSION\}/thehivemcp-\$\{VERSION\}-([a-z]+-[a-z0-9]+)\.tar\.gz$`)
	docExtract = regexp.MustCompile(`tar xzf "thehivemcp-\$\{VERSION\}-([a-z]+-[a-z0-9]+)\.tar\.gz"`)
	docRename  = regexp.MustCompile(`mv (\S+) thehivemcp\b`)
)

// docPages are the pages a user installs from: the README and everything under docs/.
func docPages(t *testing.T) map[string]string {
	t.Helper()

	pages := map[string]string{}
	add := func(path string) {
		content, err := os.ReadFile(path) // #nosec G304 -- a page of this repository
		require.NoError(t, err)

		pages[path] = string(content)
	}

	add(filepath.Join(repoRoot, "README.md"))
	require.NoError(t, filepath.WalkDir(filepath.Join(repoRoot, "docs"), func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".md") {
			add(path)
		}

		return err
	}))

	return pages
}

// archiveName is the archive Tarballs writes for target at version.
func archiveName(t *testing.T, version, target string) string {
	t.Helper()

	dir := t.TempDir()
	build := filepath.Join(dir, "build")
	require.NoError(t, os.MkdirAll(build, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(build, release.Binary(name, target)), []byte(elf), 0o755)) // #nosec G306 -- a test binary

	written, err := release.Tarballs(release.Options{Version: version, Build: build, Dist: filepath.Join(dir, "dist"), Name: name, Targets: []string{target}})
	require.NoError(t, err)
	require.Len(t, written, 1)

	return filepath.Base(written[0])
}

// A download link in the docs names an archive a release publishes: the versioned .tar.gz that
// Tarballs writes, never a releases/latest/download/<binary> URL, which no release has.
func TestDocsDownloadLinksNameAPublishedArchive(t *testing.T) {
	t.Parallel()

	found := 0

	for path, content := range docPages(t) {
		for _, m := range releaseURL.FindAllStringSubmatch(content, -1) {
			found++

			asset := docAsset.FindStringSubmatch(m[1])
			if !assert.NotNil(t, asset, "%s: %s is not a versioned release archive (download/${VERSION}/thehivemcp-${VERSION}-<target>.tar.gz)", path, m[0]) {
				continue
			}

			want := archiveName(t, "${VERSION}", asset[1])
			assert.Equal(t, want, filepath.Base(m[1]), "%s: %s", path, m[0])
		}
	}

	require.Positive(t, found, "no release download link found in the docs: the regexp no longer matches them")
}

// After extracting an archive, the docs rename the binary it holds, in the same code block: by
// the name the release gives it, so the mv finds the file.
func TestDocsRenameTheBinaryTheArchiveHolds(t *testing.T) {
	t.Parallel()

	found := 0

	for path, content := range docPages(t) {
		for _, loc := range docExtract.FindAllStringSubmatchIndex(content, -1) {
			found++

			target := content[loc[2]:loc[3]]
			block, _, _ := strings.Cut(content[loc[1]:], "```") // the rest of the extraction's code block
			rename := docRename.FindStringSubmatch(block)

			if assert.NotNil(t, rename, "%s: no mv of the extracted binary after extracting %s", path, target) {
				assert.Equal(t, release.Binary(name, target), rename[1], "%s: the %s archive holds another binary name", path, target)
			}
		}
	}

	require.Positive(t, found, "no archive extraction found in the docs: the regexp no longer matches them")
}

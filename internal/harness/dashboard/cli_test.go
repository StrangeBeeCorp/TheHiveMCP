package dashboard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"
)

// The command writes the page where it is asked, even when sources are missing, and says which;
// a command line it cannot run is a usage error. Not parallel: it sets PATH, so that no gh is
// found.
func TestTheCommandWritesThePageAndSaysWhatItCouldNotRead(t *testing.T) {
	git, err := exec.LookPath("git")
	require.NoError(t, err, "git is needed: run the tests through make test")
	t.Setenv("PATH", filepath.Dir(git)) // git's directory only: no gh
	t.Setenv(EnvJiraURL, "")

	dir := gittest.Repo(t, map[string]string{"a.go": "package a\n"})

	var out strings.Builder

	require.NoError(t, Command(t.Context(), dir, []string{"--weeks", "4", "--out", "page.html"}, &out))

	page, err := os.ReadFile(filepath.Join(dir, "page.html")) // #nosec G304 -- the test's own page
	require.NoError(t, err)
	assert.Contains(t, string(page), "repository health")
	assert.Contains(t, out.String(), "dashboard: "+filepath.Join(dir, "page.html"))
	assert.Contains(t, out.String(), "not read: "+SourceGitHub+": "+errNoGH.Error())
	assert.Contains(t, out.String(), "not read: "+SourceTree, "the scratch repository has no limits")

	err = Command(t.Context(), dir, []string{"--out", "page.html", "--open"}, &out)
	require.Error(t, err, "no browser to open it in a test")
	assert.Contains(t, err.Error(), "open the page")

	for _, args := range [][]string{{"--weeks", "0"}, {"--nope"}, {"extra"}} {
		require.ErrorIs(t, Command(t.Context(), dir, args, &out), ErrUsage, args)
	}
}

package evidence

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// make pr keeps the profile its own run wrote, under its commit; an older profile is another
// run's, and none is kept.
func TestTheRunKeepsOnlyTheProfileItWrote(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	head := "0123456789abcdef"

	kept, err := KeepProfile(root, head, time.Now())
	require.NoError(t, err)
	assert.False(t, kept, "no profile")

	src := filepath.Join(root, CoverageProfile)
	require.NoError(t, os.WriteFile(src, []byte("mode: set\n"), 0o600))

	kept, err = KeepProfile(root, head, time.Now().Add(time.Hour))
	require.NoError(t, err)
	assert.False(t, kept, "written before the run started")

	data, err := Profile(root, head)
	require.NoError(t, err)
	assert.Nil(t, data)

	kept, err = KeepProfile(root, head, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	assert.True(t, kept)

	data, err = Profile(root, head)
	require.NoError(t, err)
	assert.Equal(t, "mode: set\n", string(data))
}

func TestAProfileThatCannotBeKeptOrReadIsAnError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, CoverageProfile), 0o750))

	_, err := KeepProfile(root, "abc", time.Time{})
	require.Error(t, err, "the profile is a directory")

	require.NoError(t, os.MkdirAll(profilePath(root, "abc"), 0o750))

	_, err = Profile(root, "abc")
	require.Error(t, err, "the kept profile is a directory")

	root = t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".cache"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, CoverageProfile), []byte("x"), 0o600))

	_, err = KeepProfile(root, "abc", time.Time{})
	require.Error(t, err, ".cache is taken")
}

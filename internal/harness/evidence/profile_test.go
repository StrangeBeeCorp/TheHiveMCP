package evidence

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// make pr clears the profile an earlier run left, then keeps the one its own gates wrote under its
// commit; with none written, none is kept.
func TestTheRunKeepsOnlyTheProfileItWrote(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	head := "0123456789abcdef"
	src := filepath.Join(root, CoverageProfile)

	require.NoError(t, os.WriteFile(src, []byte("mode: set\nstale\n"), 0o600))
	require.NoError(t, ClearProfile(root))
	require.NoError(t, ClearProfile(root), "nothing left to clear")

	kept, err := KeepProfile(root, head)
	require.NoError(t, err)
	assert.False(t, kept, "the earlier run's profile is gone")

	data, err := Profile(root, head)
	require.NoError(t, err)
	assert.Nil(t, data)

	require.NoError(t, os.WriteFile(src, []byte("mode: set\n"), 0o600))

	kept, err = KeepProfile(root, head)
	require.NoError(t, err)
	assert.True(t, kept)

	data, err = Profile(root, head)
	require.NoError(t, err)
	assert.Equal(t, "mode: set\n", string(data))
}

func TestAProfileThatCannotBeKeptReadOrClearedIsAnError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, CoverageProfile, "x"), 0o750))

	_, err := KeepProfile(root, "abc")
	require.Error(t, err, "the profile is a directory")
	require.Error(t, ClearProfile(root), "a directory that is not empty")

	require.NoError(t, os.MkdirAll(profilePath(root, "abc"), 0o750))

	_, err = Profile(root, "abc")
	require.Error(t, err, "the kept profile is a directory")

	root = t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".cache"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, CoverageProfile), []byte("x"), 0o600))

	_, err = KeepProfile(root, "abc")
	require.Error(t, err, ".cache is taken")
}

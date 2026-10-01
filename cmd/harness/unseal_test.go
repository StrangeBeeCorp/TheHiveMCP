package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
)

const ticketFlag = "--ticket"

// sealed writes files under .claude/sealed/<ticket>/ of a fresh checkout and returns it.
func sealed(t *testing.T, ticket string, files ...string) env {
	t.Helper()

	root := t.TempDir()
	for _, f := range files {
		p := filepath.Join(root, policy.SealedDir, ticket, f)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
		require.NoError(t, os.WriteFile(p, []byte("package x\n"), 0o600))
	}

	return env{root: root}
}

func TestUnsealMovesTheSealedTestsIntoTheCode(t *testing.T) {
	t.Parallel()

	e := sealed(t, "DL-1234", "internal/tools/search/sealed_test.go", "internal/tools/search/testdata/case.json")

	var out strings.Builder

	require.NoError(t, e.unseal([]string{ticketFlag, "DL-1234"}, &out))
	assert.FileExists(t, filepath.Join(e.root, "internal/tools/search/sealed_test.go"))
	assert.FileExists(t, filepath.Join(e.root, "internal/tools/search/testdata/case.json"))
	assert.NoDirExists(t, filepath.Join(e.root, policy.SealedDir, "DL-1234"))
	assert.Contains(t, out.String(), "2 sealed test file(s) of DL-1234 are in the code")
}

func TestUnsealMovesNothingUnlessEveryFileIsANewTest(t *testing.T) {
	t.Parallel()

	code := sealed(t, "DL-1", "internal/x_test.go", "internal/x.go")
	err := code.unseal([]string{ticketFlag, "DL-1"}, &strings.Builder{})
	require.ErrorIs(t, err, errUnseal)
	assert.Contains(t, err.Error(), "internal/x.go is not a test file")
	assert.NoFileExists(t, filepath.Join(code.root, "internal/x_test.go"), "nothing moved")

	clash := sealed(t, "DL-2", "internal/y_test.go")
	require.NoError(t, os.MkdirAll(filepath.Join(clash.root, "internal"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(clash.root, "internal/y_test.go"), []byte("package x\n"), 0o600))
	err = clash.unseal([]string{ticketFlag, "DL-2"}, &strings.Builder{})
	require.ErrorIs(t, err, errUnseal)
	assert.Contains(t, err.Error(), "already exists")

	empty := sealed(t, "DL-3")
	require.ErrorIs(t, empty.unseal([]string{ticketFlag, "DL-3"}, &strings.Builder{}), errUnseal)
}

func TestUnsealTakesOnlyATicketKey(t *testing.T) {
	t.Parallel()

	e := sealed(t, "DL-4", "a_test.go")

	for _, bad := range []string{"", "../DL-4", "dl-4", "DL-4/../x"} {
		var usage usageError
		require.ErrorAs(t, e.unseal([]string{ticketFlag, bad}, &strings.Builder{}), &usage, "ticket %q", bad)
	}
}

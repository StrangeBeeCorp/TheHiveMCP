package policy_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
)

// A letter that a filesystem ignoring case reads as a plain one could spell a guarded name past
// the refusals, which compare in ASCII case only: such a path is refused, a plain accent is not.
func TestALookAlikeLetterIsRefused(t *testing.T) {
	t.Parallel()

	for _, p := range []string{
		"gates/limit\u017f.yaml", // the long s, U+017F
		"\u212aeys.go",           // the Kelvin sign, U+212A
		"stra\u00dfe.go",         // the sharp s, U+00DF
		"\u1e9e.go",              // the capital sharp s, U+1E9E
		"\ufb00ile.go",           // the ff ligature, U+FB00, the first Latin ligature
		"\ufb06ack.go",           // the st ligature, U+FB06, the last
		".claude/sealed/DL-1/\ufb01le_test.go",
	} {
		d := policy.Write(policy.Main, root, p)
		assert.True(t, d.Deny, p)
		assert.Contains(t, d.Reason, "plain letters", p)
	}

	for _, p := range []string{"caf\u00e9.go", "na\u00efve.md", "\ufb07x.go"} {
		assert.False(t, policy.Write(policy.Main, root, p).Deny, "%s has no letter that reads as a plain one", p)
	}

	assert.True(t, policy.Read(policy.Coder, root, "gates/canarie\u017f/x.patch").Deny, "nor does a read get past the canaries that way")
}

// The checkout is found by any spelling of its path, and only at a path boundary: a sibling whose
// name starts like it, its parent and a shorter path are outside it.
func TestTheCheckoutIsFoundInAnyCase(t *testing.T) {
	t.Parallel()

	inside := policy.Write(policy.Tester, root, "/R/TheHiveMCP/internal/x.go")
	assert.True(t, inside.Deny)
	assert.Contains(t, inside.Reason, "internal/x.go is code", "judged as a file of the checkout")

	whole := policy.Write(policy.Tester, root, "/R/THEHIVEMCP")
	assert.True(t, whole.Deny)
	assert.Contains(t, whole.Reason, "the Tester writes only tests", "the checkout itself, spelt in capitals")

	for _, p := range []string{"/r/thehivemcpx/y_test.go", "/R/TheHiveMCPx/y_test.go", "/r", "/r/thehivem"} {
		d := policy.Write(policy.Tester, root, p)
		assert.True(t, d.Deny, p)
		assert.Contains(t, d.Reason, "agents write inside their checkout", "%s is outside the checkout", p)
	}
}

// A role says what it is: an agent or the main session, a side of the duel, a reader.
func TestTheRolesSayWhatTheyAre(t *testing.T) {
	t.Parallel()

	for role, want := range map[policy.Role]struct {
		agent, duel, readOnly bool
		name                  string
	}{
		policy.Main:     {name: "main session"},
		policy.Tester:   {agent: true, duel: true, name: "Tester"},
		policy.Coder:    {agent: true, duel: true, name: "Coder"},
		policy.Reviewer: {agent: true, readOnly: true, name: "reviewer"},
		policy.Other:    {agent: true, name: "agent"},
	} {
		assert.Equal(t, want.agent, role.Agent(), want.name)
		assert.Equal(t, want.duel, role.Duel(), want.name)
		assert.Equal(t, want.readOnly, role.ReadOnly(), want.name)
		assert.Equal(t, want.name, role.String())
	}

	assert.Equal(t, "role 9", policy.Role(9).String())
	assert.Equal(t, policy.Main, policy.RoleOf("main"))
}

// An agent writes outside its checkout only in a temporary directory: the system's, or the one
// TMPDIR names, and only inside it.
func TestTMPDIRIsATemporaryDirectory(t *testing.T) {
	t.Setenv("TMPDIR", "/custom/scratch")

	assert.False(t, policy.Write(policy.Coder, root, "/custom/scratch/x.txt").Deny)

	for _, p := range []string{"/custom/scratchy/x.txt", "/custom/other/x.txt"} {
		assert.True(t, policy.Write(policy.Coder, root, p).Deny, p)
	}
}

// ~/ is the home directory: a file there is outside the checkout, not a directory named ~ in it.
func TestTildeIsTheHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	assert.False(t, policy.Write(policy.Tester, root, "~/notes.txt").Deny, "a temporary home is a temporary directory")
	assert.True(t, policy.Write(policy.Tester, root, "notes.txt").Deny, "the same name in the checkout is code")
}

// git's and gh's configuration is a person's wherever it lives: under $XDG_CONFIG_HOME, under
// $GH_CONFIG_DIR, and under ~/.config through a link. The directories themselves count; the rest
// of those places does not.
func TestGitAndGhConfigurationIsAPersonsWhereverItLives(t *testing.T) {
	base := t.TempDir()
	home, linked, xdg, gh := filepath.Join(base, "home"), filepath.Join(base, "linked"), filepath.Join(base, "xdg"), filepath.Join(base, "ghcfg")

	for _, d := range []string{home, linked, xdg, gh} {
		require.NoError(t, os.MkdirAll(d, 0o750))
	}

	require.NoError(t, os.Symlink(linked, filepath.Join(home, ".config")))
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("GH_CONFIG_DIR", gh)

	for _, p := range []string{
		filepath.Join(xdg, "git", "config"),
		filepath.Join(xdg, "gh"),
		filepath.Join(gh, "hosts.yml"),
		filepath.Join(linked, "git", "config"),
	} {
		d := policy.Write(policy.Main, root, p)
		assert.True(t, d.Deny, p)
		assert.Contains(t, d.Reason, "git's or gh's configuration", p)
	}

	for _, p := range []string{filepath.Join(xdg, "notes.md"), filepath.Join(linked, "other", "x")} {
		assert.False(t, policy.Write(policy.Main, root, p).Deny, p)
	}
}

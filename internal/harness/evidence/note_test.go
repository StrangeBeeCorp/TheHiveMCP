package evidence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"
)

// A clone's notes ref is named by an id drawn at random once and kept in its git config: the same
// clone keeps it, another clone draws its own, and neither names the machine. Memory is read from
// /proc/meminfo on Linux, from hw.memsize on macOS.
func TestAClonesNotesRefIsItsOwnAndTheMachineIsMeasured(t *testing.T) {
	t.Parallel()

	first, second := gittest.Repo(t, nil), gittest.Repo(t, nil)

	ref := NotesRef(t.Context(), first)
	assert.Regexp(t, `^refs/notes/harness/[0-9a-f]{16}$`, ref)
	assert.Equal(t, ref, NotesRef(t.Context(), first), "the same clone, the same ref")
	assert.Equal(t, ref, NotesPrefix+gittest.Git(t, first, "config", "--get", notesIDKey), "kept in the clone's config")
	assert.NotEqual(t, ref, NotesRef(t.Context(), second), "another clone, another ref")
	assert.Empty(t, NotesRef(t.Context(), t.TempDir()), "no repository, no id kept: no ref")

	host, _ := os.Hostname()
	if host != "" {
		assert.NotContains(t, ref, host)
	}

	p := filepath.Join(t.TempDir(), "meminfo")
	require.NoError(t, os.WriteFile(p, []byte("MemFree:  1000 kB\nMemTotal:       8152000 kB\n"), 0o600))
	assert.Equal(t, 8, linuxMemoryGB(p), "8 152 000 kB is about 8 GB")
	assert.Equal(t, 0, linuxMemoryGB(filepath.Join(t.TempDir(), "absent")))
	assert.Equal(t, 16, memsizeGB("17179869184\n"), "macOS's hw.memsize is in bytes")
	assert.Equal(t, 0, memsizeGB("unknown"))

	m := ThisMachine(t.Context())
	assert.Positive(t, m.CPUs)
	assert.NotEmpty(t, m.OS)
}

// A make pr run's note carries what the dashboard charts: when, on which machine, and each gate's
// verdict and seconds; not its logs, summaries or paths, which stay on the machine.
func TestANoteCarriesTheGateTimesAndTheMachine(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, 10, 7, 16, 0, 0, 0, time.UTC)
	ev := Evidence{
		Head: "abc123", Branch: "feat/x", Verdict: runner.Pass, Started: started, Finished: started.Add(573 * time.Second),
		Results: []runner.Result{
			{Gate: "integration", Verdict: runner.Pass, Seconds: 347.5, Summary: "DONE 1007 tests", Log: "/home/runner/run.log"},
			{Gate: "secrets", Verdict: runner.Skipped, Seconds: 0},
		},
	}
	machine := Machine{OS: "darwin", Arch: "arm64", CPUs: 15, MemoryGB: 24, CI: false}

	data, err := ev.Note(machine)
	require.NoError(t, err)

	var n Note
	require.NoError(t, json.Unmarshal(data, &n))
	assert.Equal(t, NoteSchema, n.Schema)
	assert.Equal(t, "abc123", n.Head)
	assert.Equal(t, runner.Pass, n.Verdict)
	assert.InDelta(t, 573, n.Seconds, 1e-9)
	assert.Equal(t, machine, n.Machine)
	assert.Equal(t, []GateTime{{Gate: "integration", Verdict: runner.Pass, Seconds: 347.5}, {Gate: "secrets", Verdict: runner.Skipped}}, n.Gates)
	assert.NotContains(t, string(data), "/home/runner", "no path leaves the machine")
	assert.NotContains(t, string(data), "1007 tests", "no summary either")
}

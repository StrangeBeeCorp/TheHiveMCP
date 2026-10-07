package evidence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"
)

// A machine's notes ref is named by a hash of its host name, which it does not publish; its memory
// is read from /proc/meminfo on Linux.
func TestAMachineIsNamedByAHashAndMeasured(t *testing.T) {
	t.Parallel()

	ref := NotesRef()
	assert.Regexp(t, `^refs/notes/harness/[0-9a-f]{16}$`, ref)
	assert.Equal(t, ref, NotesRef(), "the same machine, the same ref")

	host, _ := os.Hostname()
	if host != "" {
		assert.NotContains(t, ref, host)
	}

	p := filepath.Join(t.TempDir(), "meminfo")
	require.NoError(t, os.WriteFile(p, []byte("MemFree:  1000 kB\nMemTotal:       8152000 kB\n"), 0o600))
	assert.Equal(t, 8, linuxMemoryGB(p), "8 152 000 kB is about 8 GB")
	assert.Equal(t, 0, linuxMemoryGB(filepath.Join(t.TempDir(), "absent")))

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

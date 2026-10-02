//go:build unix

package runner_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/registry"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"
)

// A gate stopped by its budget, or by an interrupted run, takes the processes it started with it:
// a shell's background child does not outlive the gate.
func TestAStoppedGateStopsWhatItStarted(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		budget      int
		interruptIn time.Duration
		verdict     runner.Verdict
		summary     string
	}{
		"over its budget": {budget: 1, verdict: runner.Fail, summary: "over its 1 s budget"},
		"interrupted":     {budget: 60, interruptIn: 300 * time.Millisecond, verdict: runner.Unavailable, summary: "interrupted"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			pidFile := filepath.Join(dir, "child.pid")

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			if tc.interruptIn > 0 {
				time.AfterFunc(tc.interruptIn, cancel)
			}

			started := time.Now()
			results, err := runner.Run(ctx, &registry.Registry{}, []registry.Gate{gate("forks", "sleep 30 & echo $! > "+pidFile+"; wait", tc.budget)},
				runner.Options{Root: dir, Cadence: stop, LogDir: dir})
			require.NoError(t, err)
			assert.Less(t, time.Since(started), 15*time.Second, "the gate stops instead of waiting for its child")
			assert.Equal(t, tc.verdict, results[0].Verdict)
			assert.Contains(t, results[0].Summary, tc.summary)

			data, err := os.ReadFile(pidFile) // #nosec G304 -- the test's own temporary file
			require.NoError(t, err)

			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			require.NoError(t, err)

			assert.Eventually(t, func() bool { return !alive(pid) }, 10*time.Second, 100*time.Millisecond, "the background child is gone")
		})
	}
}

// alive reports whether a process still runs. A zombie has stopped: it only waits for its parent
// to read its status, and in a container an orphan's new parent is often a PID 1 that never does.
func alive(pid int) bool {
	if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
		return false
	}

	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)) // Linux; elsewhere, kill alone decides
	if err != nil {
		return true
	}

	// "<pid> (<command>) <state> …": the state follows the last parenthesis.
	fields := strings.Fields(string(stat[bytes.LastIndexByte(stat, ')')+1:]))

	return len(fields) == 0 || fields[0] != "Z"
}

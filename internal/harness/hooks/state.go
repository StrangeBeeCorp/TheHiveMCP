package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// statePath is the hooks' memory in a checkout, next to the run logs (git-ignored).
const statePath = ".cache/harness/state.json"

const (
	dirPerm  = 0o750
	filePerm = 0o600
)

// State is what the hooks remember between calls in one checkout.
type State struct {
	// Rounds counts the red stops of each agent since its last green one.
	Rounds map[string]int `json:"red_rounds,omitempty"`
	// RedSince is when each agent's current red streak began: its BLOCKED.md must be newer.
	RedSince map[string]time.Time `json:"red_since,omitempty"`
	// Last is the last check-fast result and the checkout it judged.
	Last *Last `json:"last,omitempty"`
	// Progress is what PreCompact saved for after the compaction.
	Progress string `json:"progress,omitempty"`
}

// Last is a check-fast result with the fingerprint of what it judged.
type Last struct {
	Check

	Fingerprint string    `json:"fingerprint"`
	Static      bool      `json:"static"`
	At          time.Time `json:"at"`
}

func load(root string) State {
	var st State

	data, err := os.ReadFile(filepath.Join(root, statePath)) // #nosec G304 -- this checkout's own state file
	if err == nil {
		_ = json.Unmarshal(data, &st)
	}

	if st.Rounds == nil {
		st.Rounds = map[string]int{}
	}

	if st.RedSince == nil {
		st.RedSince = map[string]time.Time{}
	}

	return st
}

func (st State) save(root string) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the hook state: %w", err)
	}

	p := filepath.Join(root, statePath)

	err = os.WriteFile(p, data, filePerm)
	if err != nil {
		return fmt.Errorf("write %s: %w", statePath, err)
	}

	return nil
}

// update changes the state under an exclusive lock: agents of one checkout may stop together.
func update(root string, change func(*State) error) error {
	dir := filepath.Join(root, filepath.Dir(statePath))

	err := os.MkdirAll(dir, dirPerm)
	if err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	unlock, err := lock(filepath.Join(dir, "state.lock"))
	if err != nil {
		return err
	}
	defer unlock()

	st := load(root)

	err = change(&st)
	if err != nil {
		return err
	}

	return st.save(root)
}

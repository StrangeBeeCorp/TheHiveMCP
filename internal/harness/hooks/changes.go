package hooks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
)

// The policy reads no shell command: what an agent may change is judged on what it changed. Its
// first tool call keeps the checkout as the agent found it, and its stop compares: a file its role
// may not write, however it was written, or a commit by a role that does not commit, is reported
// back like a red check-fast, with the command that restores it, until the agent undoes it.

// Snapshot is the checkout as an agent found it: HEAD, each file a commit would carry and each
// sealed test with the hash of its content (gitx.Contents), and the files HEAD does not hold as
// they are (gitx.Dirty), which git cannot restore.
type Snapshot struct {
	Head  string            `json:"head"`
	Files map[string]string `json:"files"`
	Dirty []string          `json:"dirty,omitempty"`
	// Error is why the checkout could not be read: the stop says so, and the next call tries again.
	Error string `json:"error,omitempty"`
}

// Lister reads the checkout as it is now.
type Lister func(ctx context.Context) (Snapshot, error)

// snapshotDir holds one snapshot per agent, and beside it the copies of the files its role may
// not write that git cannot restore, next to the hooks' state (git-ignored).
const snapshotDir = ".cache/harness/agents"

// shortSHA is how much of a commit a report shows.
const shortSHA = 7

// agentDir is where an agent's snapshot and copies live, relative to the checkout.
func agentDir(actor string) string {
	sum := sha256.Sum256([]byte(actor))
	return filepath.Join(snapshotDir, hex.EncodeToString(sum[:8]))
}

func snapshotPath(root, actor string) string {
	return filepath.Join(root, agentDir(actor)+".json")
}

// Remember keeps the checkout as an agent finds it, at its first tool call, with a copy of each
// file its role may not write that git cannot restore (a test the Tester has not committed yet,
// for the Coder). A checkout it could not read is kept as that error, and a snapshot that does not
// read back is taken again, at the next call. A snapshot gone since the agent started is not
// taken again: a new one would hide what the agent changed before, so it stays gone, the agent's
// calls are refused and its stop is no pass. The main session changes what it likes: it keeps
// none.
func Remember(ctx context.Context, root string, in Input, list Lister) {
	path := snapshotPath(root, in.Actor())
	if !in.Role().Agent() || usable(path) {
		return
	}

	_, statErr := os.Stat(path)
	if errors.Is(statErr, os.ErrNotExist) && load(root).Started[in.Actor()] {
		return
	}

	s, err := list(ctx)
	if err != nil {
		s = Snapshot{Error: err.Error()}
	}

	for _, rel := range s.Dirty {
		if policy.Write(in.Role(), root, rel).Deny {
			keep(root, in.Actor(), rel)
		}
	}

	data, err := json.Marshal(s)
	if err != nil || os.MkdirAll(filepath.Dir(path), dirPerm) != nil {
		return
	}

	// Written whole or not at all: a stop never reads half a snapshot.
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, filePerm) == nil && os.Rename(tmp, path) == nil {
		_ = update(root, func(st *State) error {
			st.Started[in.Actor()] = true
			return nil
		})
	}
}

// Remembered reports whether an agent's start is on record: its snapshot (of the checkout, or of
// the error that kept it from being read) and the state's note that it started. Without both its
// stop could not judge what it changed, so its tool calls are refused (hook pre-tool-use). The
// main session keeps none.
func Remembered(root string, in Input) bool {
	if !in.Role().Agent() {
		return true
	}

	_, err := os.Stat(snapshotPath(root, in.Actor()))

	return err == nil && load(root).Started[in.Actor()]
}

// readSnapshot is the checkout as the agent found it, and whether there is one: an agent that made
// no tool call has none, and changed nothing. A snapshot gone since the agent's first call, one
// that does not read back, or one of a checkout that could not be read, is an error: what the
// agent changed cannot be judged.
func readSnapshot(root string, in Input) (Snapshot, bool, error) {
	data, err := os.ReadFile(snapshotPath(root, in.Actor()))
	if errors.Is(err, os.ErrNotExist) {
		if load(root).Started[in.Actor()] {
			return Snapshot{}, false, errors.New("the snapshot of the checkout the agent started from is gone (deleted since its first tool call)")
		}

		return Snapshot{}, false, nil
	} else if err != nil {
		return Snapshot{}, false, fmt.Errorf("read the agent's snapshot: %w", err)
	}

	var s Snapshot

	err = json.Unmarshal(data, &s)
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("read the agent's snapshot: %w", err)
	}

	if s.Error != "" {
		return Snapshot{}, false, fmt.Errorf("the checkout could not be read when the agent started: %s", s.Error)
	}

	return s, true, nil
}

// NotRemembered is why an agent's tool call is refused when its start is not on record.
const NotRemembered = "harness: the checkout as you found it is not on record (" + snapshotDir + " could not be written, " +
	"or the snapshot was deleted), so your stop could not judge what you change: no tool call runs. Stop, and say so in " +
	"your answer: the session that started you must look at it"

// usable reports whether the snapshot at path reads back and holds a checkout.
func usable(path string) bool {
	data, err := os.ReadFile(path) // #nosec G304 -- this checkout's own snapshot
	if err != nil {
		return false
	}

	var s Snapshot

	return json.Unmarshal(data, &s) == nil && s.Error == ""
}

// keep copies rel into the agent's directory, when it exists.
func keep(root, actor, rel string) {
	content, err := os.ReadFile(filepath.Join(root, rel)) // #nosec G304 -- a file of this checkout
	if err != nil {
		return
	}

	kept := filepath.Join(root, agentDir(actor), rel)
	if os.MkdirAll(filepath.Dir(kept), dirPerm) == nil {
		_ = os.WriteFile(kept, content, filePerm) // #nosec G703 -- rel is a path git listed in this checkout
	}
}

// Changes are what an agent changed since its first tool call that its role may not: each file
// the role may not write (policy.Write), and a commit by the Tester, the Coder or a reviewer, each
// with the command that undoes it. A baseline is changed by its make target, which an agent runs:
// it is not judged here.
func Changes(ctx context.Context, root string, in Input, list Lister) ([]string, error) {
	before, found, err := readSnapshot(root, in)
	if err != nil || !found {
		return nil, err
	}

	now, err := list(ctx)
	if err != nil {
		return nil, err
	}

	role := in.Role()

	var out []string

	if now.Head != before.Head && (role.Duel() || role.ReadOnly()) {
		out = append(out, fmt.Sprintf("HEAD moved from %s to %s: the %s does not commit, the session that runs it does. "+
			"Undo the commit and keep its changes: git reset --soft %s", short(before.Head), short(now.Head), role, before.Head))
	}

	for _, rel := range changed(before.Files, now.Files) {
		if class, ok := policy.ClassOf(rel); ok && class == policy.Baselines {
			continue
		}

		d := policy.Write(role, root, rel)
		if !d.Deny {
			continue
		}

		reason := d.Reason
		if !strings.Contains(reason, rel) {
			reason = rel + ": " + reason
		}

		out = append(out, reason+" Restore it: "+restore(root, in.Actor(), rel, before))
	}

	return out, nil
}

// Touched are the files an agent changed since its first tool call, and of them the ones it added:
// none for an agent with no snapshot, or one the checkout could not be read for.
func Touched(ctx context.Context, root string, in Input, list Lister) (changedFiles, added []string) {
	before, found, err := readSnapshot(root, in)
	if err != nil || !found {
		return nil, nil
	}

	now, err := list(ctx)
	if err != nil {
		return nil, nil
	}

	for _, rel := range changed(before.Files, now.Files) {
		if _, ok := now.Files[rel]; !ok {
			continue // removed: nothing left to split
		}

		changedFiles = append(changedFiles, rel)

		if _, ok := before.Files[rel]; !ok {
			added = append(added, rel)
		}
	}

	return changedFiles, added
}

// restore is the command that puts rel back as the agent found it: the copy kept of a file git
// cannot restore, git's own for one HEAD holds, and a removal for a file that was not there.
func restore(root, actor, rel string, before Snapshot) string {
	kept := filepath.Join(agentDir(actor), rel)

	switch {
	case exists(filepath.Join(root, kept)):
		return fmt.Sprintf("mkdir -p %s && cp %s %s", filepath.Dir(rel), kept, rel)
	case before.Files[rel] != "" && !slices.Contains(before.Dirty, rel):
		return "git checkout HEAD -- " + rel
	case before.Files[rel] == "":
		return "rm " + rel
	}

	return "ask the session to restore it: no copy was kept"
}

// Forget drops an agent's snapshot and copies once its stop is green: a later run starts from what
// it finds.
func Forget(root string, in Input) {
	_ = os.Remove(snapshotPath(root, in.Actor()))
	_ = os.RemoveAll(filepath.Join(root, agentDir(in.Actor())))
}

// changed are the files whose content differs between two listings, added and removed ones too.
func changed(before, now map[string]string) []string {
	var out []string

	for p, sum := range now {
		if before[p] != sum {
			out = append(out, p)
		}
	}

	for p := range before {
		if _, ok := now[p]; !ok {
			out = append(out, p)
		}
	}

	sort.Strings(out)

	return out
}

// changesReport is what an agent is told about the changes its role may not make.
func changesReport(changes []string) string {
	return "you changed what your role may not change, whatever command did it:\n- " + strings.Join(changes, "\n- ") +
		"\nRun each restore command above before you stop."
}

func short(sha string) string {
	if len(sha) > shortSHA {
		return sha[:shortSHA]
	}

	return sha
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

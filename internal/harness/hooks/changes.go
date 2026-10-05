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
// read back is taken again, at the next call. The main session changes what it likes: it keeps
// none.
func Remember(ctx context.Context, root string, in Input, list Lister) {
	path := snapshotPath(root, in.Actor())
	if !in.Role().Agent() || usable(path) {
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
	if os.WriteFile(tmp, data, filePerm) == nil {
		_ = os.Rename(tmp, path)
	}
}

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
	data, err := os.ReadFile(snapshotPath(root, in.Actor()))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil // no snapshot: the agent made no tool call, so it changed nothing
	} else if err != nil {
		return nil, fmt.Errorf("read the agent's snapshot: %w", err)
	}

	var before Snapshot

	err = json.Unmarshal(data, &before)
	if err != nil {
		return nil, fmt.Errorf("read the agent's snapshot: %w", err)
	}

	if before.Error != "" {
		return nil, fmt.Errorf("the checkout could not be read when the agent started: %s", before.Error)
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

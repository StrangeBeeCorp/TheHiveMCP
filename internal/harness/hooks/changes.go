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
	"sort"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
)

// The policy reads no shell command: what an agent may change is judged on what it changed. Its
// first tool call keeps the checkout as the agent found it, and its stop compares: a file its role
// may not write, however it was written, or a commit by a role that does not commit, is reported
// back like a red check-fast until the agent undoes it.

// Snapshot is the checkout as an agent found it: HEAD, and each file a commit would carry with
// the hash of its content (gitx.Contents).
type Snapshot struct {
	Head  string            `json:"head"`
	Files map[string]string `json:"files"`
}

// Lister reads the checkout as it is now.
type Lister func(ctx context.Context) (Snapshot, error)

// snapshotDir holds one snapshot per agent, next to the hooks' state (git-ignored).
const snapshotDir = ".cache/harness/agents"

// shortSHA is how much of a commit a report shows.
const shortSHA = 7

func snapshotPath(root, actor string) string {
	sum := sha256.Sum256([]byte(actor))
	return filepath.Join(root, snapshotDir, hex.EncodeToString(sum[:8])+".json")
}

// Remember keeps the checkout as an agent finds it, at its first tool call. The main session
// changes what it likes: it keeps none.
func Remember(ctx context.Context, root string, in Input, list Lister) {
	path := snapshotPath(root, in.Actor())
	if !in.Role().Agent() || exists(path) {
		return
	}

	s, err := list(ctx)
	if err != nil {
		return
	}

	data, err := json.Marshal(s)
	if err != nil || os.MkdirAll(filepath.Dir(path), dirPerm) != nil {
		return
	}

	_ = os.WriteFile(path, data, filePerm)
}

// Changes are what an agent changed since its first tool call that its role may not: each file
// the role may not write (policy.Write), and a commit by the Tester, the Coder or a reviewer. A
// baseline is changed by its make target, which an agent runs: it is not judged here.
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

	now, err := list(ctx)
	if err != nil {
		return nil, err
	}

	role := in.Role()

	var out []string

	if now.Head != before.Head && (role.Duel() || role.ReadOnly()) {
		out = append(out, fmt.Sprintf("HEAD moved from %s to %s: the %s does not commit, the session that runs it does",
			short(before.Head), short(now.Head), role))
	}

	for _, rel := range changed(before.Files, now.Files) {
		if class, ok := policy.ClassOf(rel); ok && class == policy.Baselines {
			continue
		}

		if d := policy.Write(role, root, rel); d.Deny && strings.Contains(d.Reason, rel) {
			out = append(out, d.Reason)
		} else if d.Deny {
			out = append(out, rel+": "+d.Reason)
		}
	}

	return out, nil
}

// Forget drops an agent's snapshot once its stop is green: a later run starts from what it finds.
func Forget(root string, in Input) {
	_ = os.Remove(snapshotPath(root, in.Actor()))
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
		"\nUndo these changes (git checkout -- FILE restores a tracked file; delete a file you added) before you stop."
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

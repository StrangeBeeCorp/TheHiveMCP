package dashboard

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/evidence"
)

// fetchedNotes is where the page fetches origin's notes: apart from this clone's own, which a
// fetch must never overwrite (a run not pushed yet keeps its note there).
const fetchedNotes = "refs/harness/dashboard-notes/"

// NoteRun is one make pr run, as its note tells it.
type NoteRun struct {
	evidence.Note

	// Machine is the machine's description, which the page groups the runs by.
	MachineLabel string
}

// readNotes fetches origin's notes and reads them with this clone's own, each run once. A fetch
// that fails leaves this clone's notes, and says so.
func readNotes(ctx context.Context, root string) ([]NoteRun, error) {
	git := func(stdin io.Reader, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- git with the dashboard's own arguments
		cmd.Dir = root
		cmd.WaitDelay = waitDelay
		cmd.Stdin = stdin

		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("git %s: %w", args[0], err)
		}

		return out, nil
	}

	_, fetchErr := git(nil, "fetch", "-q", "origin", "+"+evidence.NotesPrefix+"*:"+fetchedNotes+"*")

	refs, err := git(nil, "for-each-ref", "--format=%(refname)", strings.TrimSuffix(evidence.NotesPrefix, "/"), strings.TrimSuffix(fetchedNotes, "/"))
	if err != nil {
		return nil, err
	}

	var blobs []string

	// A notes ref is a commit whose tree holds one blob per noted commit (in fan-out directories
	// once there are many): ls-tree reads it wherever the ref lives, which git notes does not.
	for ref := range strings.FieldsSeq(string(refs)) {
		list, err := git(nil, "ls-tree", "-r", ref)
		if err != nil {
			return nil, err
		}

		for line := range strings.SplitSeq(strings.TrimSpace(string(list)), "\n") {
			if f := strings.Fields(line); len(f) == 4 && f[1] == "blob" { // "<mode> blob <sha>\t<path>"
				blobs = append(blobs, f[2])
			}
		}
	}

	runs, err := readBlobs(func(in io.Reader) ([]byte, error) { return git(in, "cat-file", "--batch") }, blobs)
	if err != nil {
		return nil, err
	}

	if fetchErr != nil {
		return runs, fmt.Errorf("origin's notes could not be fetched (%w): only this clone's are shown", fetchErr)
	}

	return runs, nil
}

// readBlobs reads the notes' blobs in one git cat-file --batch, each line of a note one run, and
// keeps each run once (the same note may sit under this clone's ref and origin's).
func readBlobs(catFile func(io.Reader) ([]byte, error), blobs []string) ([]NoteRun, error) {
	if len(blobs) == 0 {
		return nil, nil
	}

	out, err := catFile(strings.NewReader(strings.Join(blobs, "\n") + "\n"))
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}

	var runs []NoteRun

	r := bufio.NewReader(bytes.NewReader(out))

	for {
		body, err := nextBlob(r)
		if errors.Is(err, io.EOF) {
			return runs, nil
		}

		if err != nil {
			return nil, fmt.Errorf("read the notes: %w", err)
		}

		for line := range bytes.SplitSeq(body, []byte("\n")) {
			var n evidence.Note
			if json.Unmarshal(line, &n) != nil || n.Schema != evidence.NoteSchema {
				continue
			}

			if key := n.Head + n.Started.Format(time.RFC3339Nano); !seen[key] {
				seen[key] = true

				runs = append(runs, NoteRun{Note: n, MachineLabel: machineLabel(n.Machine)})
			}
		}
	}
}

// batchFields are those of a blob's header in git cat-file --batch's output.
const batchFields = 3

// nextBlob reads the next blob of git cat-file --batch's output: nil for one git did not find
// ("<sha> missing"), io.EOF at the end.
func nextBlob(r *bufio.Reader) ([]byte, error) {
	header, err := r.ReadString('\n')
	if err != nil {
		return nil, err //nolint:wrapcheck // io.EOF, which the caller tests for
	}

	f := strings.Fields(header)
	if len(f) != batchFields { // "<sha> <type> <size>"
		return nil, nil
	}

	size, err := strconv.Atoi(f[2])
	if err != nil {
		return nil, fmt.Errorf("a blob's size: %w", err)
	}

	body := make([]byte, size+1) // the content and its newline

	_, err = io.ReadFull(r, body)
	if err != nil {
		return nil, fmt.Errorf("a blob of %d bytes: %w", size, err)
	}

	return body, nil
}

// machineLabel describes a machine the way the page groups the runs: "darwin/arm64, 15 CPUs, 24 GB".
func machineLabel(m evidence.Machine) string {
	s := fmt.Sprintf("%s/%s, %d CPUs", m.OS, m.Arch, m.CPUs)
	if m.MemoryGB > 0 {
		s += fmt.Sprintf(", %d GB", m.MemoryGB)
	}

	if m.CI {
		s = "CI " + s
	}

	return s
}

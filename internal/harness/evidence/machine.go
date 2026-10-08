package evidence

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
)

// NotesPrefix is where make pr's notes live: one ref per clone, refs/notes/harness/<its id>, so
// that two clones rarely write the same ref, and merge it when they do (gitx.PushNotes).
const NotesPrefix = "refs/notes/harness/"

// notesIDKey is the git config key a clone's notes id is kept under, drawn once at random: it
// names no machine and no person.
const notesIDKey = "harness.notes-id"

const (
	bytesPerGB = 1 << 30
	kbPerGB    = 1 << 20
	notesIDLen = 8 // random bytes of a clone's notes id
)

// ThisMachine is the machine make pr runs on.
func ThisMachine(ctx context.Context) Machine {
	return Machine{OS: runtime.GOOS, Arch: runtime.GOARCH, CPUs: runtime.NumCPU(), MemoryGB: memoryGB(ctx), CI: os.Getenv("CI") != ""}
}

// NotesRef is this clone's notes ref, named by its notes id, drawn at random the first time and
// kept in its git config. "" when the id can be neither read nor kept.
func NotesRef(ctx context.Context, root string) string {
	id := gitx.ConfigValue(ctx, root, notesIDKey)
	if id == "" {
		b := make([]byte, notesIDLen)

		_, err := rand.Read(b)
		if err != nil {
			return ""
		}

		id = hex.EncodeToString(b)

		if gitx.SetConfig(ctx, root, notesIDKey, id) != nil {
			return ""
		}
	}

	return NotesPrefix + id
}

// memoryGB is the machine's memory, rounded to the GB; 0 when it cannot be read.
func memoryGB(ctx context.Context) int {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.CommandContext(ctx, "sysctl", "-n", "hw.memsize").Output()
		if err != nil {
			return 0
		}

		return memsizeGB(string(out))
	case "linux":
		return linuxMemoryGB("/proc/meminfo")
	}

	return 0
}

// memsizeGB reads macOS's hw.memsize, in bytes.
func memsizeGB(s string) int {
	b, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}

	return int((b + bytesPerGB/2) / bytesPerGB)
}

// linuxMemoryGB reads MemTotal from a /proc/meminfo, in kB.
func linuxMemoryGB(p string) int {
	f, err := os.Open(p) // #nosec G304 -- the kernel's own file
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) >= 2 && f[0] == "MemTotal:" {
			kb, err := strconv.ParseInt(f[1], 10, 64)
			if err == nil {
				return int((kb + kbPerGB/2) / kbPerGB)
			}
		}
	}

	return 0
}

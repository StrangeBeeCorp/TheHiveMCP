package evidence

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// NotesPrefix is where make pr's notes live: one ref per machine, refs/notes/harness/<machine>,
// so that two machines never push over each other's notes.
const NotesPrefix = "refs/notes/harness/"

const (
	bytesPerGB = 1 << 30
	kbPerGB    = 1 << 20
	machineID  = 8 // bytes of the host name's hash a machine's ref is named by
)

// ThisMachine is the machine make pr runs on.
func ThisMachine(ctx context.Context) Machine {
	return Machine{OS: runtime.GOOS, Arch: runtime.GOARCH, CPUs: runtime.NumCPU(), MemoryGB: memoryGB(ctx), CI: os.Getenv("CI") != ""}
}

// NotesRef is this machine's notes ref: named by a hash of the host name, which it does not publish.
func NotesRef() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}

	sum := sha256.Sum256([]byte(host))

	return NotesPrefix + hex.EncodeToString(sum[:machineID])
}

// memoryGB is the machine's memory, rounded to the GB; 0 when it cannot be read.
func memoryGB(ctx context.Context) int {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.CommandContext(ctx, "sysctl", "-n", "hw.memsize").Output()
		if err != nil {
			return 0
		}

		b, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		if err != nil {
			return 0
		}

		return int((b + bytesPerGB/2) / bytesPerGB)
	case "linux":
		return linuxMemoryGB("/proc/meminfo")
	}

	return 0
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

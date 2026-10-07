package evidence

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"
)

// NoteSchema is the version of a make pr run's git note, for the dashboard that reads them.
const NoteSchema = 1

// Note is what make pr shares of a run (git notes, refs/notes/harness/<machine>): when, where, and
// how long each gate took. Logs, summaries and paths stay on the machine.
type Note struct {
	Schema  int            `json:"schema"`
	Head    string         `json:"head"`
	Branch  string         `json:"branch"`
	Verdict runner.Verdict `json:"verdict"`
	Started time.Time      `json:"started"`
	Seconds float64        `json:"seconds"`
	Machine Machine        `json:"machine"`
	Gates   []GateTime     `json:"gates"`
}

// GateTime is one gate's verdict and time.
type GateTime struct {
	Gate    string         `json:"gate"`
	Verdict runner.Verdict `json:"verdict"`
	Seconds float64        `json:"seconds,omitempty"`
}

// Machine is what the timings depend on: the platform, the processors and the memory, and
// whether it is CI.
type Machine struct {
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	CPUs     int    `json:"cpus"`
	MemoryGB int    `json:"memory_gb,omitempty"`
	CI       bool   `json:"ci"`
}

// Note is the run's note, as JSON.
func (ev Evidence) Note(m Machine) ([]byte, error) {
	n := Note{
		Schema: NoteSchema, Head: ev.Head, Branch: ev.Branch, Verdict: ev.Verdict, Started: ev.Started,
		Seconds: ev.Finished.Sub(ev.Started).Seconds(), Machine: m, Gates: make([]GateTime, 0, len(ev.Results)),
	}

	for _, r := range ev.Results {
		n.Gates = append(n.Gates, GateTime{Gate: r.Gate, Verdict: r.Verdict, Seconds: r.Seconds})
	}

	data, err := json.Marshal(n)
	if err != nil {
		return nil, fmt.Errorf("encode the run's note: %w", err)
	}

	return data, nil
}

package dashboard

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"
)

// errNoRun means this clone has not run make pr: coverage and mutation come from its logs.
var errNoRun = errors.New("no make pr run in .cache/harness/runs: run make pr")

// LocalRun is what this clone's last make pr measured, the logs the notes leave out.
type LocalRun struct {
	Head    string
	Started time.Time
	Verdict runner.Verdict
	// Coverage is the total statement coverage, -1 when the run has none.
	Coverage float64
	Mutation []Mutation
}

// Mutation is a package's mutation score against its floor.
type Mutation struct {
	Package  string
	Efficacy float64
	Floor    float64
}

var (
	coverageTotal = regexp.MustCompile(`^total:\s+\(statements\)\s+([\d.]+)%`)
	mutationLine  = regexp.MustCompile(`^(\S+): efficacy ([\d.]+)% \(floor ([\d.]+)%\)`)
)

// readLocal reads the last pr run of this clone: its record, its coverage total and its mutation
// scores.
func readLocal(root string) (LocalRun, error) {
	runs, err := filepath.Glob(filepath.Join(root, ".cache", "harness", "runs", "pr-*", "results.json"))
	if err != nil || len(runs) == 0 {
		return LocalRun{Coverage: -1}, errNoRun
	}

	slices.Sort(runs) // named pr-<date>-<time>: the last is the latest

	dir := filepath.Dir(runs[len(runs)-1])

	data, err := os.ReadFile(runs[len(runs)-1]) // #nosec G304 -- this clone's own cache
	if err != nil {
		return LocalRun{Coverage: -1}, fmt.Errorf("read the last make pr: %w", err)
	}

	var rec runner.Record

	err = json.Unmarshal(data, &rec)
	if err != nil {
		return LocalRun{Coverage: -1}, fmt.Errorf("read the last make pr: %w", err)
	}

	r := LocalRun{Head: rec.Head, Started: rec.Started, Verdict: rec.Verdict, Coverage: -1}

	if total, ok := CoverageOf(filepath.Join(dir, "coverage.log")); ok {
		r.Coverage = total
	}

	scanLog(filepath.Join(dir, "mutation.log"), func(line string) {
		if m := mutationLine.FindStringSubmatch(line); m != nil {
			efficacy, _ := strconv.ParseFloat(m[2], 64)
			floor, _ := strconv.ParseFloat(m[3], 64)
			r.Mutation = append(r.Mutation, Mutation{Package: m[1], Efficacy: efficacy, Floor: floor})
		}
	})

	return r, nil
}

// CoverageOf is the total statement coverage a coverage log prints; false when it prints none.
func CoverageOf(log string) (float64, bool) {
	total := -1.0

	scanLog(log, func(line string) {
		if m := coverageTotal.FindStringSubmatch(line); m != nil {
			total, _ = strconv.ParseFloat(m[1], 64)
		}
	})

	return total, total >= 0
}

// scanLog calls each for every line of a log, however long (a test's output may print a schema on
// one line); a log that is not there has no line.
func scanLog(p string, each func(string)) {
	f, err := os.Open(p) // #nosec G304 -- this clone's own cache
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	r := bufio.NewReader(f)

	for {
		line, err := r.ReadString('\n')
		if line != "" {
			each(strings.TrimRight(line, "\r\n"))
		}

		if err != nil {
			return
		}
	}
}

// Package bench reads `go test -bench` output and holds the benchmarks to gates/bench.yaml:
// against the same benchmarks run at the base on the same machine, and against absolute budgets.
package bench

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Max is an absolute budget: the most a benchmark may take or allocate per op, whatever the base.
type Max struct {
	Ns     float64 `yaml:"ns"`
	Allocs float64 `yaml:"allocs"`
}

// Budgets is gates/bench.yaml.
type Budgets struct {
	Packages  []string `yaml:"packages"`
	Count     int      `yaml:"count"`
	Benchtime string   `yaml:"benchtime"`
	// Slower is how many times slower than at the base a benchmark may get (median ns/op).
	Slower float64 `yaml:"slower"`
	// MoreAllocs is the share of the base's allocations per op a benchmark may add, at least one.
	MoreAllocs float64        `yaml:"more_allocs"`
	Max        map[string]Max `yaml:"max"`
}

// ErrBudgets means gates/bench.yaml is incomplete.
var ErrBudgets = errors.New("bench budgets")

// Load reads and checks gates/bench.yaml.
func Load(path string) (Budgets, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- the repository's own budgets file
	if err != nil {
		return Budgets{}, fmt.Errorf("read %s: %w", path, err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var b Budgets

	err = dec.Decode(&b)
	if err != nil {
		return Budgets{}, fmt.Errorf("parse %s: %w", path, err)
	}

	if len(b.Packages) == 0 || b.Count < 1 || b.Benchtime == "" || b.Slower < 1 || b.MoreAllocs < 0 {
		return Budgets{}, fmt.Errorf("%w: %s needs packages, count ≥ 1, benchtime, slower ≥ 1 and more_allocs ≥ 0", ErrBudgets, path)
	}

	return b, nil
}

// Args are the go test arguments that run the benchmarks and nothing else.
func (b Budgets) Args() []string {
	args := []string{"test", "-run", "^NoTest", "-bench", ".", "-benchmem", "-count", strconv.Itoa(b.Count), "-benchtime", b.Benchtime}
	return append(args, b.Packages...)
}

// Result is a benchmark's medians over its runs.
type Result struct {
	Runs   int
	Ns     float64
	Bytes  float64
	Allocs float64
}

// line is one result line of go test -bench -benchmem, its GOMAXPROCS suffix dropped.
var line = regexp.MustCompile(`^(Benchmark\S+?)(?:-\d+)?\s+\d+\s+([\d.]+) ns/op(?:\s+([\d.]+) B/op\s+([\d.]+) allocs/op)?`)

// Parse reads go test -bench output: the median of each benchmark's runs.
func Parse(out string) map[string]Result {
	samples := map[string][][3]float64{}

	for text := range strings.SplitSeq(out, "\n") {
		m := line.FindStringSubmatch(strings.TrimSpace(text))
		if m == nil {
			continue
		}

		var s [3]float64
		for i, v := range m[2:] {
			s[i], _ = strconv.ParseFloat(v, 64)
		}

		samples[m[1]] = append(samples[m[1]], s)
	}

	results := map[string]Result{}

	for name, runs := range samples {
		r := Result{Runs: len(runs)}
		r.Ns, r.Bytes, r.Allocs = median(runs, 0), median(runs, 1), median(runs, 2)
		results[name] = r
	}

	return results
}

func median(runs [][3]float64, i int) float64 {
	values := make([]float64, len(runs))
	for j, r := range runs {
		values[j] = r[i]
	}

	slices.Sort(values)

	mid := len(values) / 2
	if len(values)%2 == 1 {
		return values[mid]
	}

	return (values[mid-1] + values[mid]) / 2
}

// Compare lists the benchmarks over budget: slower or allocating more than at the base (when the
// base has them), or over their absolute budget, and the budgets whose benchmark did not run.
func Compare(base, head map[string]Result, b Budgets) []string {
	var problems []string

	for name, h := range head {
		if was, ok := base[name]; ok {
			problems = append(problems, againstBase(name, was, h, b)...)
		}

		if limit, ok := b.Max[name]; ok {
			problems = append(problems, againstMax(name, h, limit)...)
		}
	}

	for name := range b.Max {
		if _, ok := head[name]; !ok {
			problems = append(problems, name+": has a budget in gates/bench.yaml but did not run: rename or remove its budget")
		}
	}

	sort.Strings(problems)

	return problems
}

func againstBase(name string, was, now Result, b Budgets) []string {
	var problems []string

	if was.Ns > 0 && now.Ns > was.Ns*b.Slower {
		problems = append(problems, fmt.Sprintf("%s: %.0f ns/op, %.2fx the base's %.0f (budget %.2fx)", name, now.Ns, now.Ns/was.Ns, was.Ns, b.Slower))
	}

	if extra := math.Max(1, was.Allocs*b.MoreAllocs); now.Allocs > was.Allocs+extra {
		problems = append(problems, fmt.Sprintf("%s: allocates %.0f times per op, the base %.0f (budget +%.0f)", name, now.Allocs, was.Allocs, extra))
	}

	return problems
}

func againstMax(name string, now Result, limit Max) []string {
	var problems []string

	if limit.Ns > 0 && now.Ns > limit.Ns {
		problems = append(problems, fmt.Sprintf("%s: %.0f ns/op, over its budget of %.0f", name, now.Ns, limit.Ns))
	}

	if limit.Allocs > 0 && now.Allocs > limit.Allocs {
		problems = append(problems, fmt.Sprintf("%s: allocates %.0f times per op, over its budget of %.0f", name, now.Allocs, limit.Allocs))
	}

	return problems
}

// Table is one line per benchmark: HEAD's medians, and the base's when it has the benchmark.
func Table(base, head map[string]Result) []string {
	names := make([]string, 0, len(head))
	for name := range head {
		names = append(names, name)
	}

	sort.Strings(names)

	lines := make([]string, 0, len(names))

	for _, name := range names {
		h := head[name]
		row := fmt.Sprintf("%-30s %12.0f ns/op %10.0f B/op %8.0f allocs/op", name, h.Ns, h.Bytes, h.Allocs)

		if was, ok := base[name]; ok && was.Ns > 0 {
			row += fmt.Sprintf("   base %12.0f ns/op %8.0f allocs/op (%.2fx)", was.Ns, was.Allocs, h.Ns/was.Ns)
		}

		lines = append(lines, row)
	}

	return lines
}

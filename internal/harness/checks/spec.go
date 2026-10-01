package checks

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/registry"
)

const (
	// SpecPath holds the spec items: what the server promises a model, each checked.
	SpecPath = "spec/items.yaml"
	// changelogPath gets an entry for every change to what the model reads.
	changelogPath = "CHANGELOG.md"
	// surfacePrefix is the committed snapshot of what the model reads (make surface-propose).
	surfacePrefix = "baselines/surface/"
	// SurfaceSummary is a file every snapshot has: a base without it had no snapshot yet.
	SurfaceSummary = surfacePrefix + "tokens.json"
	statusActive   = "active"
)

var (
	specID  = regexp.MustCompile(`^[A-Z]{3}-[0-9]{3}$`)
	specTag = regexp.MustCompile(`//\s*spec:\s*([A-Z0-9, -]+)`)
)

// SpecItem is one promise, with what proves it: tests named file:Test, tests tagged
// `// spec: ID`, and gates.
type SpecItem struct {
	ID     string   `yaml:"id"`
	Title  string   `yaml:"title"`
	Rule   string   `yaml:"rule"`
	Status string   `yaml:"status"`
	Tests  []string `yaml:"tests"`
	Gates  []string `yaml:"gates"`
}

// Spec is spec/items.yaml.
type Spec struct {
	Items []SpecItem `yaml:"items"`
}

// ParseSpec decodes the spec, rejecting unknown keys.
func ParseSpec(data []byte) (Spec, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var s Spec

	err := dec.Decode(&s)
	if err != nil {
		return Spec{}, fmt.Errorf("parse %s: %w", SpecPath, err)
	}

	err = registry.TrailingComments(data)
	if err != nil {
		return Spec{}, fmt.Errorf("%s: %w", SpecPath, err)
	}

	return s, nil
}

// SpecTraces checks that every active spec item is proven by something that exists: a test it
// names, a test tagged with its id, or a gate of the registry; and that no tag names an item that
// does not exist.
func SpecTraces(tree Tree, reg *registry.Registry) ([]Finding, error) {
	data, err := tree.Read(SpecPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("spec: %w", err)
	}

	spec, err := ParseSpec(data)
	if err != nil {
		return nil, err
	}

	tags, out, err := specTags(tree, spec)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}

	for _, item := range spec.Items {
		out = append(out, itemFindings(tree, reg, item, seen, tags[item.ID])...)
	}

	return out, nil
}

func itemFindings(tree Tree, reg *registry.Registry, item SpecItem, seen map[string]bool, tagged int) []Finding {
	var out []Finding

	bad := func(msg string) { out = append(out, Finding{Path: SpecPath, Msg: item.ID + ": " + msg}) }

	switch {
	case !specID.MatchString(item.ID):
		bad("an id is three capitals, a dash and three digits (SIL-001)")
	case seen[item.ID]:
		bad("defined twice")
	case strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.Rule) == "":
		bad("needs a title and a rule")
	case item.Status != statusActive && item.Status != "draft":
		bad("status is active or draft")
	}

	seen[item.ID] = true

	for _, ref := range item.Tests {
		if !testExists(tree, ref) {
			bad(fmt.Sprintf("names %s, which is not a test of the repository (path_test.go:TestName)", ref))
		}
	}

	for _, g := range item.Gates {
		if _, ok := reg.Gate(g); !ok {
			bad(fmt.Sprintf("names gate %q, which gates/registry.yaml does not define", g))
		}
	}

	if item.Status == statusActive && len(item.Tests)+len(item.Gates)+tagged == 0 {
		bad("is active but nothing proves it: name a test or a gate, or tag a test `// spec: " + item.ID + "`")
	}

	return out
}

// specTags counts the `// spec: ID` tags per item, and reports those naming no item.
func specTags(tree Tree, spec Spec) (map[string]int, []Finding, error) {
	known := map[string]bool{}
	for _, item := range spec.Items {
		known[item.ID] = true
	}

	tags := map[string]int{}

	var out []Finding

	for _, f := range tree.Files() {
		if !strings.HasSuffix(f, "_test.go") {
			continue
		}

		content, err := tree.Read(f)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}

		if err != nil {
			return nil, nil, fmt.Errorf("spec: %w", err)
		}

		out = append(out, fileTags(f, content, known, tags)...)
	}

	return tags, out, nil
}

// fileTags counts the tags of one test file into tags, and reports those naming no item.
func fileTags(f string, content []byte, known map[string]bool, tags map[string]int) []Finding {
	var out []Finding

	sc := bufio.NewScanner(bytes.NewReader(content))
	sc.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), len(content)+1)

	for line := 1; sc.Scan(); line++ {
		m := specTag.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}

		for id := range strings.SplitSeq(m[1], ",") {
			id = strings.TrimSpace(id)
			if known[id] {
				tags[id]++
			} else {
				out = append(out, Finding{Path: f, Line: line, Msg: fmt.Sprintf("tags spec item %q, which %s does not define", id, SpecPath)})
			}
		}
	}

	return out
}

// testExists reports whether ref, path_test.go:TestName, names a test function of the tree.
func testExists(tree Tree, ref string) bool {
	file, name, ok := strings.Cut(ref, ":")
	if !ok || !strings.HasSuffix(file, "_test.go") {
		return false
	}

	var params string

	switch {
	case strings.HasPrefix(name, "Test"):
		params = "(t *testing.T)"
	case strings.HasPrefix(name, "Fuzz"):
		params = "(f *testing.F)"
	default:
		return false
	}

	content, err := tree.Read(file)
	if err != nil {
		return false
	}

	return bytes.Contains(content, []byte("func "+name+params))
}

// Changelog checks that a change to what the model reads has a CHANGELOG.md entry: the surface
// gate makes every such change update baselines/surface/, so a change there without one to
// CHANGELOG.md is a model-facing change nobody announced. The change that first adds the snapshot
// (snapshotAtBase false) changes nothing a model reads.
func Changelog(changed []string, snapshotAtBase bool) []Finding {
	if !snapshotAtBase {
		return nil
	}

	var touched []string

	for _, p := range changed {
		if strings.HasPrefix(p, surfacePrefix) {
			touched = append(touched, p)
		}
	}

	if len(touched) == 0 || slices.Contains(changed, changelogPath) {
		return nil
	}

	const shown = 3
	if len(touched) > shown {
		touched = append(touched[:shown], "…")
	}

	return []Finding{{Path: changelogPath, Msg: "what the model reads changed (" + strings.Join(touched, ", ") +
		") but CHANGELOG.md did not: add an entry under ## [Unreleased] saying what changed for a model"}}
}

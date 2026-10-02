package checks

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/registry"
)

// CanariesPath lists the known-bad patches and the gate each one must make fail.
const CanariesPath = "gates/canaries/canaries.yaml"

// Canary is a known-bad patch: applied to the code, the named gate must fail. A canary that its
// gate lets through means the check stopped working.
type Canary struct {
	ID    string `yaml:"id"`
	Item  string `yaml:"item"`
	Gate  string `yaml:"gate"`
	Patch string `yaml:"patch"`
	// Run restricts a test gate to the tests that should catch it (passed as RUN to make).
	Run string `yaml:"run"`
	// Expect is a regular expression the gate's log must match: the failure is the one the canary
	// causes, not a compile error or a broken stack.
	Expect string `yaml:"expect"`
	Why    string `yaml:"why"`
}

// LoadCanaries parses the canary manifest and checks it against the registry and the spec: each
// canary names a gate that exists, an item of the spec, and a patch in gates/canaries/.
func LoadCanaries(tree Tree, reg *registry.Registry) ([]Canary, error) {
	data, err := tree.Read(CanariesPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", CanariesPath, err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var file struct {
		Canaries []Canary `yaml:"canaries"`
	}

	err = dec.Decode(&file)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", CanariesPath, err)
	}

	err = registry.TrailingComments(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", CanariesPath, err)
	}

	items := map[string]bool{}

	specData, err := tree.Read(SpecPath)
	if err == nil {
		spec, err := ParseSpec(specData)
		if err != nil {
			return nil, err
		}

		for _, it := range spec.Items {
			items[it.ID] = true
		}
	}

	var problems []string

	seen := map[string]bool{}

	for _, c := range file.Canaries {
		problems = append(problems, canaryProblems(tree, reg, c, items, seen)...)
		seen[c.ID] = true
	}

	if len(problems) > 0 {
		return nil, errors.New(CanariesPath + ": " + strings.Join(problems, "; "))
	}

	return file.Canaries, nil
}

func canaryProblems(tree Tree, reg *registry.Registry, c Canary, items, seen map[string]bool) []string {
	var p []string

	if c.ID == "" || seen[c.ID] {
		p = append(p, fmt.Sprintf("canary %q: an id is required and unique", c.ID))
	}

	if _, ok := reg.Gate(c.Gate); !ok {
		p = append(p, fmt.Sprintf("canary %q names gate %q, which the registry does not define", c.ID, c.Gate))
	}

	if !items[c.Item] {
		p = append(p, fmt.Sprintf("canary %q names item %q, which %s does not define", c.ID, c.Item, SpecPath))
	}

	if !tree.Exists("gates/canaries/" + c.Patch) {
		p = append(p, fmt.Sprintf("canary %q: no patch gates/canaries/%s", c.ID, c.Patch))
	}

	if strings.TrimSpace(c.Why) == "" {
		p = append(p, fmt.Sprintf("canary %q does not say what it breaks", c.ID))
	}

	_, err := regexp.Compile(c.Expect)
	if c.Expect == "" || err != nil {
		p = append(p, fmt.Sprintf("canary %q needs an expect: a regular expression of the failure its gate reports", c.ID))
	}

	return p
}

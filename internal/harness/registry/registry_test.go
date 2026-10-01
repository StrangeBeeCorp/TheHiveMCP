package registry_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/registry"
)

const (
	docsTree = "docs/**"
	anyTest  = "**/*_test.go"
	makefile = "Makefile"
)

func TestMatch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		pattern, name string
		want          bool
	}{
		{docsTree, "docs/reference/gates.md", true},
		{docsTree, "docs", true},
		{docsTree, "docsx/a.md", false},
		{anyTest, "internal/tools/manage/manage_test.go", true},
		{anyTest, "main_test.go", true},
		{anyTest, "internal/tools/manage/create.go", false},
		{"*.md", "README.md", true},
		{"*.md", "docs/README.md", false},
		{"internal/tools/*/models.go", "internal/tools/manage/models.go", true},
		{"internal/tools/*/models.go", "internal/tools/models.go", false},
		{"scripts/package-*.sh", "scripts/package-tarballs.sh", true},
		{makefile, makefile, true},
		{makefile, "docs/Makefile", false},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, registry.Match(c.pattern, c.name), "Match(%q, %q)", c.pattern, c.name)
	}
}

func TestTheRepositoryRegistryIsValid(t *testing.T) {
	t.Parallel()

	reg, err := registry.Load(filepath.Join("..", "..", "..", "gates", "registry.yaml"))
	require.NoError(t, err)
	assert.NotEmpty(t, reg.At("stop"), "check-fast must run something")

	profile := reg.Select([]string{"docs/reference/permissions-examples/analyst.yaml"})
	assert.True(t, profile.Has("trust"), "the example profiles are the integration tests' permission fixtures")

	for _, g := range reg.Gates {
		if strings.Contains(g.Runs, registry.ScopePlaceholder) {
			for _, c := range g.Cadence {
				assert.NotContains(t, reg.Command(g, c), registry.ScopePlaceholder, "gate %s at %s", g.ID, c)
			}
		}
	}
}

const sampleRegistry = `
scopes: {edit: e, stop: s, commit: c, pr: p, ci: i, nightly: n}
suites:
  - {id: everything, paths: [gates/**]}
  - {id: tools, paths: ["internal/tools/**"]}
  - {id: surface, paths: ["internal/tools/*/models.go"], implies: [tools]}
  - {id: docs, paths: [docs/**, "*.md"]}
gates:
  - {id: lint, runs: "lint {scope}", cadence: [stop, ci], suites: ["*"], budget_s: 9, blocks: x}
  - {id: unit, runs: "go test", cadence: [ci], suites: [tools], budget_s: 9, blocks: x}
  - {id: tokens, builtin: tokens, cadence: [stop], suites: [surface], budget_s: 9, blocks: x}
`

func load(t *testing.T, content string) (*registry.Registry, error) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "registry.yaml")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))

	reg, err := registry.Load(p)
	if err != nil {
		return nil, fmt.Errorf("load the sample: %w", err)
	}

	return reg, nil
}

func TestSelect(t *testing.T) {
	t.Parallel()
	reg, err := load(t, sampleRegistry)
	require.NoError(t, err)

	docs := reg.Select([]string{"README.md"})
	assert.False(t, docs.All)
	assert.Equal(t, []string{"docs"}, docs.Names())

	lint, _ := reg.Gate("lint")
	unit, _ := reg.Gate("unit")

	assert.True(t, docs.Selects(lint), "a * gate runs whatever changed")
	assert.False(t, docs.Selects(unit), "a docs change must not run the unit tests")

	surface := reg.Select([]string{"internal/tools/manage/models.go"})
	assert.Equal(t, []string{"surface", "tools"}, surface.Names(), "surface implies tools, and tools also claims the path")
	assert.True(t, surface.Selects(unit))

	unknown := reg.Select([]string{"somewhere/new.go"})
	assert.True(t, unknown.All, "a path no suite claims selects everything")
	assert.Contains(t, unknown.Reason, "somewhere/new.go")

	harness := reg.Select([]string{"gates/registry.yaml"})
	assert.True(t, harness.All, "a change to the everything suite selects everything")

	none := reg.Select(nil)
	assert.False(t, none.All)
	assert.Empty(t, none.Names())

	assert.Equal(t, "lint s", reg.Command(lint, "stop"))
	assert.Len(t, reg.At("ci"), 2)
}

func TestValidateReportsEveryProblem(t *testing.T) {
	t.Parallel()
	_, err := load(t, `
scopes: {stop: s}
suites:
  - {id: tools, paths: []}
  - {id: tools, paths: [x], implies: [nowhere]}
gates:
  - {id: a, runs: x, builtin: y, cadence: [sometimes], suites: [ghost], budget_s: 0}
  - {id: a, cadence: [], suites: []}
`)
	require.Error(t, err)

	for _, want := range []string{
		`suite "tools" is defined twice`,
		`suite "tools" has no paths`,
		`"everything" suite is missing`,
		`implies unknown suite "nowhere"`,
		`no scope for cadence "ci"`,
		`gate "a" is defined twice`,
		`needs exactly one of runs and builtin`,
		`gate "a" has no budget_s`,
		`does not say what blocks`,
		`unknown cadence "sometimes"`,
		`unknown suite "ghost"`,
		`gate "a" has no cadence`,
		`gate "a" has no suites`,
	} {
		assert.Contains(t, err.Error(), want)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	t.Parallel()
	_, err := load(t, strings.Replace(sampleRegistry, "budget_s: 9, blocks: x}", "budget: 9, budget_s: 9, blocks: x}", 1))
	require.Error(t, err, "a misspelt field must not silently drop a setting")
}

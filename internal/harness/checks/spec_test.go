package checks_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/registry"
)

// tag builds a spec tag without writing one in this file, which the spec gate also reads.
func tag(id string) string { return "// sp" + "ec: " + id + "\n" }

const (
	specFile = "spec/items.yaml"
	testFile = "pkg/a_test.go"
)

func specRegistry(t *testing.T) *registry.Registry {
	t.Helper()

	reg, err := registry.Parse([]byte(`
scopes: {edit: x, stop: x, commit: x, pr: x, ci: x, nightly: x}
suites: [{id: everything, paths: ["**"]}]
gates: [{id: unit, runs: make test, cadence: [ci], suites: ["*"], budget_s: 1, blocks: a test}]
`), "registry.yaml")
	require.NoError(t, err)

	return reg
}

func TestSpecTracesEveryActiveItem(t *testing.T) {
	t.Parallel()

	tree := checks.MemTree{
		specFile: []byte(`items:
  - {id: AAA-001, title: by test, rule: r, status: active, tests: ["pkg/a_test.go:TestA"]}
  - {id: BBB-001, title: by tag, rule: r, status: active}
  - {id: CCC-001, title: by gate, rule: r, status: active, gates: [unit]}
  - {id: DDD-001, title: untraced, rule: r, status: active}
  - {id: EEE-001, title: draft, rule: r, status: draft}
  - {id: FFF-001, title: bad refs, rule: r, status: active, tests: ["pkg/a_test.go:TestGone"], gates: [lint]}
`),
		testFile: []byte("package pkg\n\n" + tag("BBB-001") + "func TestA(t *testing.T) {}\n\n" + tag("ZZZ-999") + "func TestB(t *testing.T) {}\n"),
	}

	found, err := checks.SpecTraces(tree, specRegistry(t))
	require.NoError(t, err)

	msgs := messages(found)
	assert.Contains(t, msgs, `DDD-001: is active but nothing proves it`)
	assert.Contains(t, msgs, `FFF-001: names pkg/a_test.go:TestGone, which is not a test`)
	assert.Contains(t, msgs, `FFF-001: names gate "lint"`)
	assert.Contains(t, msgs, `pkg/a_test.go:6: tags spec item "ZZZ-999"`)

	for _, ok := range []string{"AAA-001", "BBB-001", "CCC-001", "EEE-001"} {
		assert.NotContains(t, msgs, ok+":", "%s is traced or a draft", ok)
	}
}

func TestSpecItemsAreWellFormed(t *testing.T) {
	t.Parallel()

	tree := checks.MemTree{specFile: []byte(`items:
  - {id: bad, title: t, rule: r, status: active, gates: [unit]}
  - {id: AAA-001, title: t, rule: r, status: done, gates: [unit]}
  - {id: AAA-001, title: "", rule: r, status: active, gates: [unit]}
`)}

	found, err := checks.SpecTraces(tree, specRegistry(t))
	require.NoError(t, err)

	msgs := messages(found)
	assert.Contains(t, msgs, "bad: an id is three capitals")
	assert.Contains(t, msgs, "AAA-001: status is active or draft")
	assert.Contains(t, msgs, "AAA-001: defined twice")
}

func TestTheRepositorySpecIsTraced(t *testing.T) {
	t.Parallel()

	reg, err := registry.Load("../../../gates/registry.yaml")
	require.NoError(t, err)

	tree := checks.DirTree{Root: "../../.."}
	data, err := tree.Read(specFile)
	require.NoError(t, err)

	spec, err := checks.ParseSpec(data)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(spec.Items), 10)

	for _, item := range spec.Items {
		for _, g := range item.Gates {
			_, ok := reg.Gate(g)
			assert.True(t, ok, "%s names gate %s", item.ID, g)
		}
	}
}

func TestAModelFacingChangeNeedsAChangelogEntry(t *testing.T) {
	t.Parallel()

	snapshot := "baselines/surface/tools/search-entities.json"

	assert.NotEmpty(t, checks.Changelog([]string{snapshot, "internal/tools/search/models.go"}, true))
	assert.Empty(t, checks.Changelog([]string{snapshot, "CHANGELOG.md"}, true))
	assert.Empty(t, checks.Changelog([]string{"internal/tools/search/handler.go"}, true), "the snapshot did not change: nothing the model reads did")
	assert.Empty(t, checks.Changelog(nil, true))
	assert.Empty(t, checks.Changelog([]string{snapshot}, false), "the first snapshot changes nothing a model reads")
}

func TestCanariesNameAGateAnItemAndAPatch(t *testing.T) {
	t.Parallel()

	tree := checks.MemTree{
		specFile: []byte("items:\n  - {id: AAA-001, title: t, rule: r, status: active, gates: [unit]}\n"),
		"gates/canaries/canaries.yaml": []byte(`canaries:
  - {id: ok, item: AAA-001, gate: unit, patch: ok.patch, why: breaks it}
  - {id: bad, item: ZZZ-001, gate: nope, patch: missing.patch, why: ""}
`),
		"gates/canaries/ok.patch": []byte("diff\n"),
	}

	_, err := checks.LoadCanaries(tree, specRegistry(t))
	require.Error(t, err)

	for _, want := range []string{`names gate "nope"`, `names item "ZZZ-001"`, "no patch gates/canaries/missing.patch", "does not say what it breaks"} {
		assert.Contains(t, err.Error(), want)
	}

	delete(tree, "gates/canaries/canaries.yaml")
	tree["gates/canaries/canaries.yaml"] = []byte("canaries:\n  - {id: ok, item: AAA-001, gate: unit, patch: ok.patch, why: breaks it}\n")

	list, err := checks.LoadCanaries(tree, specRegistry(t))
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestTheRepositoryCanariesAreWellFormed(t *testing.T) {
	t.Parallel()

	reg, err := registry.Load("../../../gates/registry.yaml")
	require.NoError(t, err)

	list, err := checks.LoadCanaries(checks.DirTree{Root: "../../.."}, reg)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(list), 10)
}

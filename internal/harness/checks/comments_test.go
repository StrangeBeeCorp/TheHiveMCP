package checks_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
)

// In an unquoted text, " #" starts a YAML comment, which would cut a rule, a limit's reason or a
// canary's purpose without a word: the spec, the limits and the canary manifest refuse it.
func TestTheHarnessFilesRefuseACommentAfterAValue(t *testing.T) {
	t.Parallel()

	const cut = "a comment after a value on line "

	_, err := checks.ParseSpec([]byte("items:\n  - id: AAA-001\n    title: t\n    rule: an entry under ## [Unreleased]\n    status: active\n"))
	require.ErrorContains(t, err, cut+"4")

	_, err = checks.ParseLimits([]byte("hooks:\n  max_red_rounds: 6 # six\n"), "limits.yaml")
	require.ErrorContains(t, err, cut+"2")

	tree := checks.MemTree{
		specFile: []byte("items:\n  - {id: AAA-001, title: t, rule: r, status: active, gates: [unit]}\n"),
		"gates/canaries/canaries.yaml": []byte("canaries:\n  - id: ok\n    item: AAA-001\n    gate: unit\n    patch: ok.patch\n" +
			"    expect: FAIL\n    why: drops the ## [Unreleased] entry\n"),
		"gates/canaries/ok.patch": []byte("diff\n"),
	}

	_, err = checks.LoadCanaries(tree, specRegistry(t))
	require.ErrorContains(t, err, cut+"7")
}

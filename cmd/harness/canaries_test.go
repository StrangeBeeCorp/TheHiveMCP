package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"
)

// stubHarness stands in for bin/harness in a scratch checkout: the canary runner calls it as
// `run --cadence nightly --gate <gate> --force`. The gate "red" fails on HEAD alone; any other
// fails, with a log, when the patch added bad.txt (the check catches it) or other.txt (it fails
// for another reason).
const stubHarness = `#!/bin/sh
gate="$5"
log=".cache/harness/runs/nightly-1/$gate.log"
mkdir -p "$(dirname "$log")"
if [ "$gate" = red ]; then echo broken > "$log"; exit 1; fi
if [ -f bad.txt ]; then echo "the check caught it" > "$log"; exit 1; fi
if [ -f other.txt ]; then echo "a compile error" > "$log"; exit 1; fi
exit 0
`

const canaryManifest = `canaries:
  - {id: caught, item: AAA-001, gate: unit, patch: bad.patch, expect: "caught it", why: the check catches it}
  - {id: escaped, item: AAA-001, gate: unit, patch: harmless.patch, expect: "caught it", why: the check misses it}
  - {id: wrong-reason, item: AAA-001, gate: unit, patch: other.patch, expect: "caught it", why: it fails for another reason}
  - {id: red-base, item: AAA-001, gate: red, patch: bad.patch, expect: "caught it", why: the gate fails without it}
  - {id: stale, item: AAA-001, gate: unit, patch: stale.patch, expect: "caught it", why: the code it broke moved}
`

// newFile is a patch that adds a one-line file.
func newFile(name string) string {
	return "diff --git a/" + name + " b/" + name + "\nnew file mode 100644\n--- /dev/null\n+++ b/" + name + "\n@@ -0,0 +1 @@\n+x\n"
}

// canaryCheckout is a repository with the stub harness, a registry with the two gates the
// canaries name, a spec item and the manifest with its patches.
func canaryCheckout(t *testing.T) string {
	t.Helper()

	dir := gittest.Repo(t, map[string]string{
		registryPath: `
scopes: {edit: a, stop: a, commit: a, pr: a, ci: a, nightly: a}
suites: [{id: everything, review: high, paths: ["**"]}]
gates:
  - {id: unit, runs: "true", cadence: [nightly], suites: ["*"], budget_s: 10, blocks: x}
  - {id: red, runs: "true", cadence: [nightly], suites: ["*"], budget_s: 10, blocks: x}
`,
		limitsPath: glueLimits, ignoreFile: ignoreCache, "README.md": "a\n",
		"spec/items.yaml":               "items:\n  - {id: AAA-001, title: t, rule: r, status: active, gates: [unit]}\n",
		"gates/canaries/canaries.yaml":  canaryManifest,
		"gates/canaries/bad.patch":      newFile("bad.txt"),
		"gates/canaries/harmless.patch": newFile("harmless.txt"),
		"gates/canaries/other.patch":    newFile("other.txt"),
		"gates/canaries/stale.patch":    "diff --git a/README.md b/README.md\n--- a/README.md\n+++ b/README.md\n@@ -1 +1 @@\n-not what it says\n+b\n",
		"bin/harness":                   stubHarness,
	})
	require.NoError(t, os.Chmod(filepath.Join(dir, "bin", "harness"), 0o700)) // #nosec G302 -- the stub must run
	gittest.Git(t, dir, "add", "-A")
	gittest.Commit(t, dir)
	t.Chdir(dir)
	t.Setenv("CI", "")

	return dir
}

// outcome is the line the report prints for a canary, its verdict first (the progress lines above
// it are indented).
func outcome(out, id string) string {
	for line := range strings.SplitSeq(out, "\n") {
		if fields := strings.Fields(line); !strings.HasPrefix(line, " ") && len(fields) > 1 && fields[1] == id {
			return line
		}
	}

	return ""
}

// Each canary's outcome comes from its gate's exit code and log, against HEAD without the patch:
// caught, escaped, failed for another reason, failing without the patch, or no longer applying.
//
//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestEachCanaryOutcomeIsTold(t *testing.T) {
	dir := canaryCheckout(t)

	out, err := harness(t, "canaries")
	require.ErrorIs(t, err, errFailed, out)

	for id, want := range map[string][]string{
		"caught":       {"PASS", "caught in"},
		"escaped":      {"FAIL", "ESCAPED: the gate passed with the patch applied"},
		"wrong-reason": {"UNAVAILABLE", `its log has no "caught it": it failed for another reason`},
		"red-base":     {"UNAVAILABLE", "the gate fails on HEAD alone (exit 1)"},
		"stale":        {"FAIL", "the patch no longer applies to HEAD: rewrite it"},
	} {
		line := outcome(out, id)
		assert.True(t, strings.HasPrefix(line, want[0]), "%s: %q", id, line)
		assert.Contains(t, line, want[1], id)
	}

	worktrees := gittest.Git(t, dir, "worktree", "list")
	assert.Len(t, strings.Split(strings.TrimSpace(worktrees), "\n"), 1, "the scratch checkouts are gone: %s", worktrees)
}

// --id runs only the canaries it names, and refuses one the manifest does not have.
//
//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestCanariesRunsOnlyTheOnesNamed(t *testing.T) {
	canaryCheckout(t)

	out, err := harness(t, "canaries", "--id", "caught")
	require.NoError(t, err, out)
	assert.NotEmpty(t, outcome(out, "caught"))
	assert.Empty(t, outcome(out, "escaped"))

	_, err = harness(t, "canaries", "--id", "cuaght")

	var usage usageError
	require.ErrorAs(t, err, &usage)
	assert.Contains(t, usage.msg, `no canary "cuaght"`)
}

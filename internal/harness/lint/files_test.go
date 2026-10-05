package lint_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/lint"
)

const (
	goFile   = "internal/x/x.go"
	shFile   = "scripts/a.sh"
	mdFile   = "docs/a.md"
	ciFile   = ".github/workflows/ci.yml"
	makefile = "Makefile"
	mkFile   = "scripts/harness.mk"
	dockerf  = "deployment/Dockerfile"
	hookFile = ".githooks/pre-commit"
	binFile  = "bin/harness"
)

// Each file goes to the tools that read it: hooks and bin/ commands are shell, a workflow is
// YAML too, Dockerfile.* and Makefile.* by their base name, and the model-facing filter doc to
// none.
func TestClassifySortsFilesByTheToolsThatReadThem(t *testing.T) {
	t.Parallel()

	f := lint.Classify([]string{
		goFile, shFile, hookFile, binFile, mdFile, "docs/b.mdx", "internal/resources/docs/filter-dsl.md",
		"gates/registry.yaml", ciFile, ".github/workflows/sub/old.yml", dockerf, "scripts/Dockerfile.mcpb",
		makefile, mkFile, "Makefile.local", "go.mod", "README", "renovate.json",
	})

	assert.Equal(t, []string{goFile}, f.Go)
	assert.Equal(t, []string{shFile, hookFile, binFile}, f.Shell)
	assert.Equal(t, []string{mdFile, "docs/b.mdx"}, f.Markdown, "the filter doc's layout is what the model reads")
	assert.Equal(t, []string{"gates/registry.yaml", ciFile, ".github/workflows/sub/old.yml"}, f.YAML)
	assert.Equal(t, []string{ciFile}, f.Workflows, "GitHub reads only the workflows directory itself")
	assert.Equal(t, []string{dockerf, "scripts/Dockerfile.mcpb"}, f.Dockerfiles)
	assert.Equal(t, []string{makefile, mkFile, "Makefile.local"}, f.Makefiles)
}

func TestSummaryCountsTheFilesByKind(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "no files", lint.Files{}.Summary())

	f := lint.Classify([]string{goFile, "internal/y.go", shFile, ciFile, makefile})
	assert.Equal(t, "2 go, 1 sh, 1 yaml, 1 Makefile", f.Summary(), "a workflow counts once, as YAML")
	assert.Equal(t, []string{ciFile, makefile, "internal/x/x.go", "internal/y.go", shFile}, f.All())
}

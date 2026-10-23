package testutils

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const thehiveClientImport = "github.com/StrangeBeeCorp/thehive4go/thehive"

// A fake that marshals the client's own model can never show that TheHive's
// answer drifted from the client: the model always decodes back what it wrote,
// and that is how #250 got through. Fakes answer with TheHive's recorded
// answers (testdata/recorded), a field overridden where a test needs one.
//
// The check reads the source of every fake under internal/testutils (a file or
// a directory whose name starts with "fake"): no thehive.Output* type may
// appear there, as a value to marshal, a slice of them, or a field that holds
// them. Decoding what the client sent (thehive.InputAction) is reading, and
// stays fine. The helpers that talk to a real TheHive (container.go, fixtures.go)
// are not fakes.
//
// spec: VER-002
func TestNoFakeAnswersWithTheClientsOutputModels(t *testing.T) {
	t.Parallel()

	scanned := map[string]bool{}

	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() && entry.Name() == "testdata" {
			return filepath.SkipDir
		}

		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		if !isFake(path) {
			return nil
		}

		scanned[filepath.ToSlash(path)] = true

		reportClientModels(t, path)

		return nil
	})
	require.NoError(t, err)

	// The scan must have reached the fakes, or it would pass for nothing.
	require.True(t, scanned["fake_cortex.go"], "the Cortex fake was not scanned: %v", scanned)
	require.True(t, scanned["fakecortex/jobs.go"], "the job fake was not scanned: %v", scanned)
}

// clientImportName is the name a file gives the thehive client package, or
// empty when it does not import it.
func clientImportName(file *ast.File) string {
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || path != thehiveClientImport {
			continue
		}

		if spec.Name != nil {
			return spec.Name.Name
		}

		return "thehive"
	}

	return ""
}

// isFake tells a fake from the helpers that talk to a real TheHive: its file
// name, or a directory above it, starts with "fake".
func isFake(path string) bool {
	for part := range strings.SplitSeq(filepath.ToSlash(path), "/") {
		if strings.HasPrefix(part, "fake") {
			return true
		}
	}

	return false
}

// reportClientModels fails the test for every thehive.Output* the file at path
// names. A dot import of the client is refused too: it would hide the prefix.
func reportClientModels(t *testing.T, path string) {
	t.Helper()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	require.NoError(t, err, path)

	alias := clientImportName(file)
	if alias == "" {
		return
	}

	if alias == "." {
		t.Errorf("%s: a fake dot-imports the client, which hides its Output models from this check", path)

		return
	}

	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		pkg, ok := selector.X.(*ast.Ident)
		if ok && pkg.Name == alias && strings.HasPrefix(selector.Sel.Name, "Output") {
			t.Errorf("%s: a fake uses %s.%s; answer with a recorded TheHive answer (RecordedAnswer) instead of the client's model",
				fset.Position(selector.Pos()), alias, selector.Sel.Name)
		}

		return true
	})
}

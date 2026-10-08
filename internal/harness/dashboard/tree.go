package dashboard

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/registry"
)

// chokepoint is the one function every integration test reaches (AGENTS.md): it starts TheHive,
// and skips the test under -short.
const chokepoint = "StartTheHiveContainer"

// largest is how many of the largest files and fullest directories the page lists.
const largest = 8

// TreeData is what the checkout's files say.
type TreeData struct {
	Layers Layers
	// Files and Dirs are the largest code files and the fullest directories, as the structure gate
	// measures them.
	Files []checks.FileSize
	Dirs  []checks.DirSize
	// NearLimit counts the code files at 90 % of their limit or over.
	NearLimit int
	Nolint    int
	TODO      int
}

// Layers counts the tests of each layer.
type Layers struct {
	Unit, Integration, Fuzz, Benchmark, Canaries int
}

func readTree(ctx context.Context, root string) (TreeData, error) {
	files, err := gitx.Files(ctx, root)
	if err != nil {
		return TreeData{}, fmt.Errorf("list the files: %w", err)
	}

	tree := checks.DirTree{Root: root, Names: files}

	limits, err := checks.LoadLimits(filepath.Join(root, "gates", "limits.yaml"))
	if err != nil {
		return TreeData{}, fmt.Errorf("the tree's limits: %w", err)
	}

	sizes, err := checks.MeasureSizes(tree, limits.Structure)
	if err != nil {
		return TreeData{}, fmt.Errorf("the tree's sizes: %w", err)
	}

	d := TreeData{
		Files: sizes.Files[:min(largest, len(sizes.Files))], Dirs: sizes.Dirs[:min(largest, len(sizes.Dirs))],
		NearLimit: nearLimit(sizes.Files),
	}

	d.Layers, err = countLayers(tree)
	if err != nil {
		return d, err
	}

	d.Layers.Canaries = canaries(root, tree)
	d.Nolint, d.TODO = waivers(tree)

	return d, nil
}

// nearLimit counts the files at 90 % of their limit, or of their cap, or over it.
func nearLimit(files []checks.FileSize) int {
	n := 0

	for _, f := range files {
		limit := max(f.Limit, f.Cap)
		if limit > 0 && f.Lines*10 >= limit*9 { // 90 %
			n++
		}
	}

	return n
}

// canaries counts the canaries the manifest holds, 0 when it cannot be read.
func canaries(root string, tree checks.Tree) int {
	reg, err := registry.Load(filepath.Join(root, "gates", "registry.yaml"))
	if err != nil {
		return 0
	}

	c, err := checks.LoadCanaries(tree, reg)
	if err != nil {
		return 0
	}

	return len(c)
}

// waivers counts the //nolint directives and the TODOs of the Go files.
func waivers(tree checks.Tree) (nolint, todo int) {
	for _, f := range tree.Files() {
		if !strings.HasSuffix(f, ".go") {
			continue
		}

		data, err := tree.Read(f)
		if err == nil {
			nolint += bytes.Count(data, []byte("//nolint"))
			todo += bytes.Count(data, []byte("TODO"))
		}
	}

	return nolint, todo
}

// goFiles are a directory's parsed Go files, its tests apart.
type goFiles struct {
	code, tests []*ast.File
}

// countLayers counts the test functions: Fuzz and Benchmark by name, and a Test as an integration
// test when its calls reach the chokepoint, through a helper of its own package or of another
// package's code (testutils').
func countLayers(tree checks.Tree) (Layers, error) {
	dirs, err := parseDirs(tree)
	if err != nil {
		return Layers{}, err
	}

	helpers := chokepointHelpers(dirs)

	var l Layers

	for _, dir := range dirs {
		reach := reaching(append(slices.Clone(dir.code), dir.tests...), helpers)

		for _, f := range dir.tests {
			for _, decl := range f.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil {
					l.add(fn.Name.Name, reach)
				}
			}
		}
	}

	return l, nil
}

// add counts a test file's function in its layer.
func (l *Layers) add(name string, reach map[string]bool) {
	switch {
	case strings.HasPrefix(name, "Fuzz"):
		l.Fuzz++
	case strings.HasPrefix(name, "Benchmark"):
		l.Benchmark++
	case !strings.HasPrefix(name, "Test") || name == "TestMain":
	case reach[name]:
		l.Integration++
	default:
		l.Unit++
	}
}

// parseDirs parses the tree's Go files by directory; a file that does not parse is left to the
// compiler to report.
func parseDirs(tree checks.Tree) (map[string]*goFiles, error) {
	dirs := map[string]*goFiles{}
	fset := token.NewFileSet()

	for _, name := range tree.Files() {
		if !strings.HasSuffix(name, ".go") {
			continue
		}

		data, err := tree.Read(name)
		if err != nil {
			return nil, fmt.Errorf("parse the tests: %w", err)
		}

		f, err := parser.ParseFile(fset, name, data, parser.SkipObjectResolution)
		if err != nil {
			continue
		}

		dir := dirs[path.Dir(name)]
		if dir == nil {
			dir = &goFiles{}
			dirs[path.Dir(name)] = dir
		}

		if strings.HasSuffix(name, "_test.go") {
			dir.tests = append(dir.tests, f)
		} else {
			dir.code = append(dir.code, f)
		}
	}

	return dirs, nil
}

// chokepointHelpers are the functions of the packages' code that reach the chokepoint, until no
// new one appears: a helper may call another package's.
func chokepointHelpers(dirs map[string]*goFiles) map[string]bool {
	helpers := map[string]bool{chokepoint: true}

	for grew := true; grew; {
		grew = false

		for _, dir := range dirs {
			for name := range reaching(dir.code, helpers) {
				if !helpers[name] {
					helpers[name], grew = true, true
				}
			}
		}
	}

	return helpers
}

// reaching is the set of the files' functions whose calls reach one of roots, directly or through
// another of the files' functions. A call is known by its name alone (pkg.F and F alike): close
// enough to sort tests into layers.
func reaching(files []*ast.File, roots map[string]bool) map[string]bool {
	calls := callGraph(files)
	reach := map[string]bool{}

	for changed := true; changed; {
		changed = false

		for name, callees := range calls {
			if !reach[name] && slices.ContainsFunc(callees, func(c string) bool { return roots[c] || reach[c] }) {
				reach[name], changed = true, true
			}
		}
	}

	return reach
}

// callGraph names, for each function of the files, the functions it calls, its closures' calls
// included.
func callGraph(files []*ast.File) map[string][]string {
	calls := map[string][]string{}

	for _, f := range files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}

			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok {
					calls[fn.Name.Name] = append(calls[fn.Name.Name], calleeName(c.Fun))
				}

				return true
			})
		}
	}

	return calls
}

func calleeName(fun ast.Expr) string {
	switch fun := fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		return fun.Sel.Name
	}

	return ""
}

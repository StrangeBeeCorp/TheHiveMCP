package checks

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// IntegrityLimits are the weakened tests a person accepted, "path_test.go:TestName" → why.
type IntegrityLimits struct {
	Allow map[string]string `yaml:"allow"`
}

// testShape is what a test function checks: its assertions, its skips and its table cases.
type testShape struct {
	path   string
	line   int
	checks int
	skips  int
	cases  int
}

// allowHint says how a person accepts a weaker test.
const allowHint = "a person accepts it in gates/limits.yaml (integrity.allow)"

// Integrity compares each changed Go test file with its version at the base. A Test or Fuzz
// function that is gone, now skips, or has fewer assertions or table cases is a finding, unless
// limits allow it. A test moved to another changed file of its package is compared there.
// before returns a file at the base, and whether it existed there.
func Integrity(tree Tree, changed []string, before func(string) ([]byte, bool), limits IntegrityLimits) ([]Finding, error) {
	was := map[string]testShape{}
	now := map[string]testShape{}

	for _, p := range changed {
		if !strings.HasSuffix(p, "_test.go") {
			continue
		}

		if old, ok := before(p); ok {
			addShapes(was, p, old)
		}

		data, err := tree.Read(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}

		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p, err)
		}

		addShapes(now, p, data)
	}

	var found []Finding

	for key, w := range was {
		name := path.Base(key)
		if _, ok := limits.Allow[w.path+":"+name]; ok {
			continue
		}

		if msg := weaker(name, w, now[key], now[key].path != ""); msg != "" {
			found = append(found, Finding{Path: w.path, Line: now[key].line, Msg: msg + ": " + allowHint})
		}
	}

	sort.Slice(found, func(i, j int) bool { return found[i].String() < found[j].String() })

	return found, nil
}

// weaker says how a test got weaker, or nothing.
func weaker(name string, was, now testShape, kept bool) string {
	switch {
	case !kept:
		return name + " is gone"
	case now.skips > was.skips:
		return fmt.Sprintf("%s now skips (%d skips, was %d)", name, now.skips, was.skips)
	case now.checks < was.checks:
		return fmt.Sprintf("%s checks less: %d assertions, was %d", name, now.checks, was.checks)
	case now.cases < was.cases:
		return fmt.Sprintf("%s has fewer cases: %d, was %d", name, now.cases, was.cases)
	}

	return ""
}

// addShapes adds the shape of each Test and Fuzz function of a file, keyed by package directory
// and name. A file that does not parse adds nothing: the compiler reports it.
func addShapes(shapes map[string]testShape, p string, src []byte) {
	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, p, src, 0)
	if err != nil {
		return
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Body == nil || !isTestName(fn.Name.Name) {
			continue
		}

		shape := testShape{path: p, line: fset.Position(fn.Pos()).Line}
		testers := map[string]bool{}
		addTesters(testers, fn.Type)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if lit, ok := n.(*ast.FuncLit); ok {
				addTesters(testers, lit.Type)
			}

			count(&shape, n, testers)

			return true
		})
		shapes[path.Join(path.Dir(p), fn.Name.Name)] = shape
	}
}

func isTestName(name string) bool {
	return strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Fuzz")
}

// countCall counts a call that checks or skips.
func countCall(shape *testShape, call *ast.CallExpr, testers map[string]bool) {
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		switch sel.Sel.Name {
		case "Skip", "Skipf", "SkipNow":
			shape.skips++
			return
		case "Error", "Errorf", "Fatal", "Fatalf", "Fail", "FailNow":
			shape.checks++
			return
		}

		if pkg, ok := sel.X.(*ast.Ident); ok && (pkg.Name == "assert" || pkg.Name == "require") {
			shape.checks++
			return
		}
	}

	if len(call.Args) > 0 {
		if arg, ok := call.Args[0].(*ast.Ident); ok && testers[arg.Name] {
			shape.checks++
		}
	}
}

// addTesters adds the names of a function's *testing.T and *testing.F parameters.
func addTesters(testers map[string]bool, fn *ast.FuncType) {
	for _, field := range fn.Params.List {
		star, ok := field.Type.(*ast.StarExpr)
		if !ok {
			continue
		}

		if sel, ok := star.X.(*ast.SelectorExpr); ok && (sel.Sel.Name == "T" || sel.Sel.Name == "F") {
			for _, name := range field.Names {
				testers[name.Name] = true
			}
		}
	}
}

// count adds what one node of a test checks: an assertion (testify's assert and require,
// t.Error, t.Fatal, t.Fail and their forms, or a helper given the test's t), a skip, or the
// elements of a slice or map literal, which are a table's cases.
func count(shape *testShape, n ast.Node, testers map[string]bool) {
	switch n := n.(type) {
	case *ast.CallExpr:
		countCall(shape, n, testers)
	case *ast.CompositeLit:
		switch n.Type.(type) {
		case *ast.ArrayType, *ast.MapType:
			shape.cases += len(n.Elts)
		}
	}
}

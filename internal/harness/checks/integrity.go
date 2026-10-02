package checks

import (
	"errors"
	"fmt"
	"go/ast"
	buildconstraint "go/build/constraint"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strings"
)

// IntegrityLimits are the weakened tests a person accepted, "path_test.go:TestName" → why.
type IntegrityLimits struct {
	Allow map[string]string `yaml:"allow"`
}

// testShape is what a test function checks: its assertions, its skips, its table cases and fuzz
// seeds, and the returns that can end it before its checks.
type testShape struct {
	path    string
	line    int
	checks  int
	skips   int
	cases   int
	returns int
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

	var found []Finding

	for _, p := range changed {
		if !strings.HasSuffix(p, "_test.go") {
			continue
		}

		f, err := compareFile(tree, p, before, limits, was, now)
		if err != nil {
			return nil, err
		}

		found = append(found, f...)
	}

	found = append(found, newTestMains(was, now, limits)...)

	for key, w := range was {
		name := path.Base(key)
		if allowed(limits, w.path, name) {
			continue
		}

		if msg := weaker(name, w, now[key], now[key].path != ""); msg != "" {
			found = append(found, Finding{Path: w.path, Line: now[key].line, Msg: msg + ": " + allowHint})
		}
	}

	sort.Slice(found, func(i, j int) bool { return found[i].String() < found[j].String() })

	return found, nil
}

// compareFile adds a changed test file's shapes, at the base and now, and refuses a build
// constraint that takes a test file out of CI's build, where the gates run it: on a new file, or one
// CI built before. A constraint that only leaves other platforms out (unix) takes nothing away.
func compareFile(tree Tree, p string, before func(string) ([]byte, bool), limits IntegrityLimits, was, now map[string]testShape) ([]Finding, error) {
	old, existed := before(p)
	if existed {
		addShapes(was, p, old)
	}

	data, err := tree.Read(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}

	addShapes(now, p, data)

	if !ciBuilds(data) && (!existed || ciBuilds(old)) && !allowed(limits, p, "build") {
		return []Finding{{Path: p, Msg: "its build constraint is now `" + constraint(data) + "`, which takes its tests out of CI's build " +
			"(linux/amd64, no cgo, no tags): " + allowHint}}, nil
	}

	return nil, nil
}

// newTestMains refuses a TestMain the base did not have: it decides whether the package's tests run.
func newTestMains(was, now map[string]testShape, limits IntegrityLimits) []Finding {
	var found []Finding

	for key, n := range now {
		if _, existed := was[key]; !existed && path.Base(key) == "TestMain" && !allowed(limits, n.path, "TestMain") {
			found = append(found, Finding{Path: n.path, Line: n.line, Msg: "a new TestMain decides whether the package's tests run: " + allowHint})
		}
	}

	return found
}

// allowed reports whether a person accepted what changed in a test file: a test by name, "build"
// for its build constraint, "TestMain".
func allowed(limits IntegrityLimits, file, what string) bool {
	_, ok := limits.Allow[file+":"+what]
	return ok
}

// ciTags are the build tags CI's tests satisfy: GOOS linux, GOARCH amd64, the gc compiler, and the
// Go releases (go1.N). CGO_ENABLED is 0 there, and no custom tag is set.
var ciTags = map[string]bool{"linux": true, "amd64": true, "unix": true, "gc": true}

// ciBuilds reports whether CI's build includes a Go file. A constraint go cannot read builds
// nothing.
func ciBuilds(src []byte) bool {
	line := constraint(src)
	if line == "" {
		return true
	}

	expr, err := buildconstraint.Parse(line)
	if err != nil {
		return false
	}

	return expr.Eval(func(tag string) bool { return ciTags[tag] || strings.HasPrefix(tag, "go1.") })
}

// constraint is a Go file's build constraint (//go:build), or "".
func constraint(src []byte) string {
	for line := range strings.SplitSeq(string(src), "\n") {
		line = strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(line, "//go:build "), strings.HasPrefix(line, "// +build "):
			return line
		case strings.HasPrefix(line, "package "):
			return ""
		}
	}

	return ""
}

// weaker says how a test got weaker, or nothing.
func weaker(name string, was, now testShape, kept bool) string {
	switch {
	case !kept:
		return name + " is gone"
	case now.skips > was.skips:
		return fmt.Sprintf("%s now skips (%d skips, was %d)", name, now.skips, was.skips)
	case now.returns > was.returns:
		return fmt.Sprintf("%s can now return before its checks (%d returns, was %d)", name, now.returns, was.returns)
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

		shape.returns = returns(fn.Body)
		shapes[path.Join(path.Dir(p), fn.Name.Name)] = shape
	}
}

func isTestName(name string) bool {
	return strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Fuzz")
}

// returns counts the return statements of a test body and of its subtests (the function literals
// given a *testing.T), not of other function literals: a return there can end the test, or the
// subtest, before its checks run.
func returns(body *ast.BlockStmt) int {
	n := 0

	ast.Inspect(body, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.FuncLit:
			testers := map[string]bool{}
			addTesters(testers, node.Type)

			return len(testers) > 0
		case *ast.ReturnStmt:
			n++
		}

		return true
	})

	return n
}

// testerFails are the methods of *testing.T that fail the test.
var testerFails = []string{"Error", "Errorf", "Fatal", "Fatalf", "Fail", "FailNow"}

// countCall counts a call that checks, skips or adds a fuzz seed.
func countCall(shape *testShape, call *ast.CallExpr, testers map[string]bool) {
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok && countMethod(shape, sel, testers) {
		return
	}

	if len(call.Args) > 0 {
		if arg, ok := call.Args[0].(*ast.Ident); ok && testers[arg.Name] {
			shape.checks++
		}
	}
}

// countMethod counts a method call that skips, checks or adds a fuzz seed, and reports whether it
// was one. t.Error and its kind count on the test's own t, not on another value (fmt.Errorf,
// err.Error).
func countMethod(shape *testShape, sel *ast.SelectorExpr, testers map[string]bool) bool {
	recv, _ := sel.X.(*ast.Ident)
	onTester := recv != nil && testers[recv.Name]

	switch {
	case sel.Sel.Name == "Skip" || sel.Sel.Name == "Skipf" || sel.Sel.Name == "SkipNow":
		shape.skips++
	case onTester && slices.Contains(testerFails, sel.Sel.Name):
		shape.checks++
	case onTester && sel.Sel.Name == "Add":
		shape.cases++
	case recv != nil && (recv.Name == "assert" || recv.Name == "require"):
		shape.checks++
	default:
		return false
	}

	return true
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

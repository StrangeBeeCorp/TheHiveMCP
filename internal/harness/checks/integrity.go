package checks

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io"
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
// seeds, and the returns that can end it before its checks. A helper is a function of a test file
// that takes the test's *testing.T, *testing.F or testing.TB.
type testShape struct {
	path    string
	line    int
	checks  int
	skips   int
	cases   int
	returns int
	helper  bool
}

// allowHint says how a person accepts a weaker test.
const allowHint = "a person accepts it in gates/limits.yaml (integrity.allow)"

// Integrity compares each changed Go test file with its version at the base. A Test or Fuzz
// function that is gone, now skips, or has fewer assertions or table cases is a finding, unless
// limits allow it, and so is a helper that is kept but checks less or skips more: its callers count
// a call to it as one assertion. A test moved to another changed file of its package is compared
// there, and reported there. before returns a file at the base, and whether it existed there.
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
		n, kept := now[key]

		if allowed(limits, w.path, name) || (kept && allowed(limits, n.path, name)) || (w.helper && !kept) {
			continue
		}

		msg := weaker(name, w, n, kept)
		if msg == "" {
			continue
		}

		at := w.path
		if kept && n.path != w.path {
			at, msg = n.path, msg+" (moved from "+w.path+")"
		}

		found = append(found, Finding{Path: at, Line: n.line, Msg: msg + ": " + allowHint})
	}

	sort.Slice(found, func(i, j int) bool { return found[i].String() < found[j].String() })

	return found, nil
}

// compareFile adds a changed test file's shapes, at the base and now, and refuses a name or a build
// constraint that takes a test file out of CI's build, where the gates run it: on a new file (a
// renamed one is new at its new path), or one CI built before. A constraint that only leaves other
// platforms out (unix) takes nothing away.
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

	if !ciBuilds(p, data) && (!existed || ciBuilds(p, old)) && !allowed(limits, p, "build") {
		why := "its build constraint is now `" + constraint(data) + "`, which"
		if !ciBuilds(p, []byte("package x\n")) {
			why = "its name, " + path.Base(p) + ","
		}

		return []Finding{{Path: p, Msg: why + " takes its tests out of CI's build (linux/amd64, no cgo, no tags): " + allowHint}}, nil
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

// ciContext is CI's build: GOOS linux, GOARCH amd64, the gc compiler and this toolchain's Go
// releases (go1.N), with CGO_ENABLED=0 and no custom tag.
var ciContext = build.Context{GOOS: "linux", GOARCH: "amd64", Compiler: "gc", ReleaseTags: build.Default.ReleaseTags}

// ciBuilds reports whether CI's build includes the Go file p, whose content is src, as go build
// decides it: by its name (a _GOOS or _GOARCH suffix, a leading _ or .) and by its build
// constraint. A file go cannot read builds nothing.
func ciBuilds(p string, src []byte) bool {
	ctx := ciContext
	ctx.OpenFile = func(string) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(src)), nil }

	ok, err := ctx.MatchFile(path.Dir(p), path.Base(p))

	return err == nil && ok
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

// addShapes adds the shape of each Test and Fuzz function of a file, and of each helper, keyed by
// package directory and name. A file that does not parse adds nothing: the compiler reports it.
func addShapes(shapes map[string]testShape, p string, src []byte) {
	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, p, src, 0)
	if err != nil {
		return
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Body == nil {
			continue
		}

		testers := map[string]bool{}
		addTesters(testers, fn.Type)

		test := isTestName(fn.Name.Name)
		if !test && len(testers) == 0 {
			continue
		}

		shape := testShape{path: p, line: fset.Position(fn.Pos()).Line, helper: !test}
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

// addTesters adds the names of a function's *testing.T, *testing.F and testing.TB parameters.
func addTesters(testers map[string]bool, fn *ast.FuncType) {
	for _, field := range fn.Params.List {
		typ, pointer := field.Type, false
		if star, ok := typ.(*ast.StarExpr); ok {
			typ, pointer = star.X, true
		}

		sel, ok := typ.(*ast.SelectorExpr)
		if !ok {
			continue
		}

		if (pointer && (sel.Sel.Name == "T" || sel.Sel.Name == "F")) || (!pointer && sel.Sel.Name == "TB") {
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

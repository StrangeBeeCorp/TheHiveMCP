package analyzers

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/types/typeutil"
)

// integrationHelpers are the testutils functions that reach a live TheHive.
var integrationHelpers = map[string]bool{
	"StartTheHiveContainer": true, "SetupTestWithCleanup": true, "NewTestClient": true, "CreateOrgClient": true,
	"GetMCPTestClient": true, "GetMCPTestClientWithPermissions": true, "CreateCaseWithTLP": true,
}

// Parallel reports t.Parallel() in an integration test. In free-license mode every integration
// test shares one organisation and must run in order: testutils.Parallel calls t.Parallel only in
// license mode, where each test has its own.
var Parallel = &analysis.Analyzer{
	Name: "parallel",
	Doc:  "an integration test calls testutils.Parallel(t), not t.Parallel()",
	Run:  runParallel,
}

func runParallel(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if !strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") {
			continue
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !strings.HasPrefix(fn.Name.Name, "Test") {
				continue
			}

			parallels, integration := testCalls(pass, fn.Body)
			if !integration {
				continue
			}

			for _, call := range parallels {
				pass.Reportf(call.Pos(), "%s reaches TheHive through testutils: call testutils.Parallel(t), not t.Parallel(), "+
					"so that it runs in order in free-license mode", fn.Name.Name)
			}
		}
	}

	return nil, nil //nolint:nilnil // an analyzer with no result
}

// testCalls finds the test's own t.Parallel() calls and whether it reaches TheHive, subtests
// included. A subtest's t.Parallel is fine: it runs alongside its siblings only, after the
// parent's body, and before the parent's cleanup.
func testCalls(pass *analysis.Pass, body *ast.BlockStmt) ([]*ast.CallExpr, bool) {
	var parallels []*ast.CallExpr

	integration := false

	ast.Inspect(body, func(n ast.Node) bool {
		if fn := callee(pass, n); fn != nil && fn.Pkg().Path() == testutilsPath && integrationHelpers[fn.Name()] {
			integration = true
		}

		return true
	})

	ast.Inspect(body, func(n ast.Node) bool {
		if _, sub := n.(*ast.FuncLit); sub {
			return false
		}

		if fn := callee(pass, n); fn != nil && fn.Pkg().Path() == "testing" && fn.Name() == "Parallel" {
			call, _ := n.(*ast.CallExpr)
			parallels = append(parallels, call)
		}

		return true
	})

	return parallels, integration
}

// callee is the function a call node calls, when it is a call to a function of some package.
func callee(pass *analysis.Pass, n ast.Node) *types.Func {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return nil
	}

	fn, _ := typeutil.Callee(pass.TypesInfo, call).(*types.Func)
	if fn == nil || fn.Pkg() == nil {
		return nil
	}

	return fn
}

// Package analyzers holds the Go analyzers of TheHiveMCP's model-facing rules. cmd/analyzers runs
// them over the module (the analyzers gate):
//
//   - untrusted: entity free text never reaches a string the server writes itself (UNT-001, UNT-002)
//   - cutflag: a result that can be cut always says whether it was (TRN-001)
//   - parallel: integration tests call testutils.Parallel, not t.Parallel
package analyzers

import (
	"go/ast"
	"go/types"
	"strings"
	"unicode"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/types/typeutil"
)

const (
	modulePath    = "github.com/StrangeBeeCorp/TheHiveMCP"
	utilsPath     = modulePath + "/internal/utils"
	toolsPath     = modulePath + "/internal/tools"
	testutilsPath = modulePath + "/internal/testutils"
	// clientPath is the generated TheHive client: its entity types carry user-written text.
	clientPath = "github.com/StrangeBeeCorp/thehive4go"
)

// freeText are the entity fields anyone with write access to TheHive can fill: they are
// [UNTRUSTED_DATA] wherever they appear, so they never go into a string the server marks as its
// own. Ids, statuses and labels from sealed enums are not here.
var freeText = map[string]bool{
	"title": true, "description": true, "summary": true, "message": true, "data": true, "tags": true,
	"name": true, "displayName": true, "report": true, "content": true, "comment": true, "text": true,
	"body": true, "details": true, "label": true, "value": true, "filename": true, "customFields": true,
}

// Untrusted reports entity free text passed to the strings the server writes itself: the
// TrustedString conversion, Trustedf, the tool errors and their hints. Those skip the
// [UNTRUSTED_DATA] wrapper, so a title there could carry instructions to the model unmarked.
var Untrusted = &analysis.Analyzer{
	Name:     "untrusted",
	Doc:      "entity free text never reaches TrustedString, Trustedf, NewToolError(f) or Hint(f) (UNT-001, UNT-002)",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      runUntrusted,
}

func runUntrusted(pass *analysis.Pass) (any, error) {
	insp, _ := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call, _ := n.(*ast.CallExpr)

		sink := sinkOf(pass, call)
		if sink == "" {
			return
		}

		for _, arg := range call.Args {
			if field := entityText(pass, arg); field != "" {
				pass.Reportf(arg.Pos(), "%s reaches %s: entity text in a string the server writes itself skips the "+
					"[UNTRUSTED_DATA] boundary (UNT-001, UNT-002); quote ids and statuses only", field, sink)
			}
		}
	})

	return nil, nil //nolint:nilnil // an analyzer with no result
}

// sinkOf names the call when it builds a string the server writes itself, else "".
func sinkOf(pass *analysis.Pass, call *ast.CallExpr) string {
	if tv, ok := pass.TypesInfo.Types[call.Fun]; ok && tv.IsType() {
		if isNamed(tv.Type, utilsPath, "TrustedString") {
			return "utils.TrustedString"
		}

		return ""
	}

	fn, _ := typeutil.Callee(pass.TypesInfo, call).(*types.Func)
	if fn == nil || fn.Pkg() == nil {
		return ""
	}

	switch path, name := fn.Pkg().Path(), fn.Name(); {
	case path == utilsPath && name == "Trustedf":
		return "utils.Trustedf"
	case path == toolsPath && (name == "NewToolError" || name == "NewToolErrorf"):
		return "tools." + name
	case path == toolsPath && (name == "Hint" || name == "Hintf"):
		return "ToolError." + name
	}

	return ""
}

// entityText finds, in an argument, a field or getter of a TheHive entity that holds free text.
func entityText(pass *analysis.Pass, arg ast.Expr) string {
	found := ""

	ast.Inspect(arg, func(n ast.Node) bool {
		if found != "" {
			return false
		}

		if _, isFunc := n.(*ast.FuncLit); isFunc {
			return false
		}

		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		s := pass.TypesInfo.Selections[sel]
		if s == nil {
			return true
		}

		named := namedOf(s.Recv())
		if named == nil || named.Obj().Pkg() == nil || !strings.HasPrefix(named.Obj().Pkg().Path(), clientPath) {
			return true
		}

		if freeText[fieldName(sel.Sel.Name)] {
			found = named.Obj().Name() + "." + sel.Sel.Name
		}

		return true
	})

	return found
}

// fieldName turns a client field or getter (Title, GetTitle, GetTitleOk) into its JSON name.
func fieldName(selector string) string {
	name := strings.TrimSuffix(strings.TrimPrefix(selector, "Get"), "Ok")
	if name == "" {
		return ""
	}

	r := []rune(name)
	r[0] = unicode.ToLower(r[0])

	return string(r)
}

func namedOf(t types.Type) *types.Named {
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}

	named, _ := t.(*types.Named)

	return named
}

func isNamed(t types.Type, pkg, name string) bool {
	named := namedOf(t)
	return named != nil && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == pkg && named.Obj().Name() == name
}

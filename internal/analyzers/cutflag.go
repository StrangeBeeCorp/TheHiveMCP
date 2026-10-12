package analyzers

import (
	"go/ast"
	"go/types"
	"reflect"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// cutFlags are the JSON names of the fields that tell a model a result was cut.
var cutFlags = map[string]bool{"hasMore": true, "truncated": true}

// CutFlag reports a result literal that leaves its hasMore or truncated field unset. A full page
// that does not say there is more reads to a model as the complete answer.
var CutFlag = &analysis.Analyzer{
	Name:     "cutflag",
	Doc:      "a struct literal of a type with a hasMore or truncated field sets that field explicitly (TRN-001)",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      runCutFlag,
}

func runCutFlag(pass *analysis.Pass) (any, error) {
	insp, _ := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	insp.Preorder([]ast.Node{(*ast.CompositeLit)(nil)}, func(n ast.Node) {
		lit, _ := n.(*ast.CompositeLit)
		// An empty literal is the zero value an error return carries: it is no page.
		if len(lit.Elts) == 0 || strings.HasSuffix(pass.Fset.Position(lit.Pos()).Filename, "_test.go") {
			return
		}

		tv, ok := pass.TypesInfo.Types[lit]
		if !ok {
			return
		}

		st, ok := tv.Type.Underlying().(*types.Struct)
		if !ok {
			return
		}

		for _, field := range flagFields(st) {
			if !sets(lit, st, field) {
				pass.Reportf(lit.Pos(), "this %s leaves %s unset: a full page would read as the complete answer (TRN-001)",
					types.TypeString(tv.Type, types.RelativeTo(pass.Pkg)), field)
			}
		}
	})

	return nil, nil //nolint:nilnil // an analyzer with no result
}

// flagFields are the Go names of a struct's bool fields whose JSON name says a result was cut.
func flagFields(st *types.Struct) []string {
	var out []string

	for i := range st.NumFields() {
		f := st.Field(i)

		name, _, _ := strings.Cut(reflect.StructTag(st.Tag(i)).Get("json"), ",")
		if cutFlags[name] && types.Identical(f.Type(), types.Typ[types.Bool]) {
			out = append(out, f.Name())
		}
	}

	return out
}

// sets reports whether the literal gives the field a value: by key, or positionally.
func sets(lit *ast.CompositeLit, st *types.Struct, field string) bool {
	if len(lit.Elts) == st.NumFields() {
		if _, keyed := lit.Elts[0].(*ast.KeyValueExpr); !keyed {
			return true
		}
	}

	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		if id, ok := kv.Key.(*ast.Ident); ok && id.Name == field {
			return true
		}
	}

	return false
}

package analyzers

import (
	"go/ast"
	"go/constant"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// taint maps the variables of a package that hold entity free text to the field it came from.
// A variable takes it when it is assigned (=, :=, var) or ranges over an expression that holds
// it, directly or through another tainted variable, and its type can carry text (a string, a
// pointer to or a collection of strings, an interface). It follows no call: text a function
// builds from its argument and returns is not seen, nor text kept in a struct field.
type taint map[types.Object]string

// tainted finds the package's tainted variables, repeating until no assignment adds one.
func tainted(pass *analysis.Pass) taint {
	t := taint{}

	for changed := true; changed; {
		changed = false

		for _, file := range pass.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				if t.flow(pass, n) {
					changed = true
				}

				return true
			})
		}
	}

	return t
}

// flow taints what the statement assigns from text, and reports whether that added a variable.
func (t taint) flow(pass *analysis.Pass, n ast.Node) bool {
	switch s := n.(type) {
	case *ast.AssignStmt:
		return t.assign(pass, s.Lhs, s.Rhs)
	case *ast.ValueSpec:
		lhs := make([]ast.Expr, len(s.Names))
		for i, name := range s.Names {
			lhs[i] = name
		}

		return t.assign(pass, lhs, s.Values)
	case *ast.RangeStmt:
		if src := t.text(pass, s.X); src != "" && s.Value != nil {
			return t.mark(pass, s.Value, src)
		}
	}

	return false
}

// assign taints each left side whose right side holds text; all of them for a call that returns
// several values from text (title, ok := a.GetTitleOk()).
func (t taint) assign(pass *analysis.Pass, lhs, rhs []ast.Expr) bool {
	added := false

	if len(rhs) == 1 && len(lhs) > 1 {
		if src := t.text(pass, rhs[0]); src != "" {
			for _, l := range lhs {
				added = t.mark(pass, l, src) || added
			}
		}

		return added
	}

	for i := range min(len(lhs), len(rhs)) {
		if src := t.text(pass, rhs[i]); src != "" {
			added = t.mark(pass, lhs[i], src) || added
		}
	}

	return added
}

// mark taints the variable an expression names, when its type can carry text.
func (t taint) mark(pass *analysis.Pass, e ast.Expr, src string) bool {
	id, ok := ast.Unparen(e).(*ast.Ident)
	if !ok || id.Name == "_" {
		return false
	}

	obj := pass.TypesInfo.ObjectOf(id)
	if obj == nil || t[obj] != "" || !carriesText(obj.Type()) {
		return false
	}

	t[obj], _, _ = strings.Cut(src, " (through ") // the field it came from, whatever the variables between

	return true
}

// text names the entity free text an expression holds, through a field, a getter or a tainted
// variable, or "".
func (t taint) text(pass *analysis.Pass, arg ast.Expr) string {
	found := ""

	ast.Inspect(arg, func(n ast.Node) bool {
		if found != "" {
			return false
		}

		switch e := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.Ident:
			if src := t[pass.TypesInfo.Uses[e]]; src != "" {
				found = src + " (through " + e.Name + ")"
			}
		case *ast.SelectorExpr:
			found = entityField(pass, e)
		case *ast.IndexExpr:
			found = rowField(pass, e)
		}

		return true
	})

	return found
}

// entityField names a field or a method of a TheHive entity that can hold text the wrapper does
// not trust, or "": a getter is judged by the field it reads, and any other method returning text
// (ToMap) as entity text.
func entityField(pass *analysis.Pass, sel *ast.SelectorExpr) string {
	s := pass.TypesInfo.Selections[sel]
	if s == nil {
		return ""
	}

	named := namedOf(s.Recv())
	if named == nil || named.Obj().Pkg() == nil || !strings.HasPrefix(named.Obj().Pkg().Path(), clientPath) {
		return ""
	}

	held := s.Type()
	if sig, ok := held.(*types.Signature); ok {
		if sig.Results().Len() == 0 {
			return ""
		}

		held = sig.Results().At(0).Type()
	}

	_, method := s.Obj().(*types.Func)
	getter := !method || strings.HasPrefix(sel.Sel.Name, "Get")

	if !carriesText(held) || (getter && trusted[jsonName(named, sel.Sel.Name)]) {
		return ""
	}

	return named.Obj().Name() + "." + sel.Sel.Name
}

// rowField names the value of a row (a map with string keys, as search and manage results are)
// read by a key the wrapper does not trust, or by a key computed at run time, or "".
func rowField(pass *analysis.Pass, ix *ast.IndexExpr) string {
	m, ok := pass.TypesInfo.TypeOf(ix.X).Underlying().(*types.Map)
	if !ok || !carriesText(m.Key()) || !carriesText(m.Elem()) {
		return ""
	}

	key := pass.TypesInfo.Types[ix.Index]
	if key.Value == nil || key.Value.Kind() != constant.String {
		return "a row read by a computed key"
	}

	if name := constant.StringVal(key.Value); !trusted[name] {
		return "row[" + strconv.Quote(name) + "]"
	}

	return ""
}

// carriesText reports whether a value of the type can hold a string.
func carriesText(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		return u.Info()&types.IsString != 0
	case *types.Pointer:
		return carriesText(u.Elem())
	case *types.Slice:
		return carriesText(u.Elem())
	case *types.Array:
		return carriesText(u.Elem())
	case *types.Map:
		return carriesText(u.Key()) || carriesText(u.Elem())
	case *types.Interface:
		return true
	}

	return false
}

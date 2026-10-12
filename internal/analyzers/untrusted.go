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
	"reflect"
	"slices"
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

// trusted are the entity fields, by their JSON names, whose values the wrapper lets reach the model
// unmarked: trustedFields and dateFields of internal/utils/parsers.go, which a test there keeps
// equal to this list. Every other entity field that can hold text is [UNTRUSTED_DATA], as the
// wrapper treats it (deny by default), so it never goes into a string the server marks as its own.
var trusted = map[string]bool{
	"_id": true, "_type": true, "_createdBy": true, "_updatedBy": true, "_kind": true, "id": true, "caseId": true,
	"patternId": true, "organisationId": true, "attachmentId": true, "rootId": true, "requestId": true, "objectId": true,
	"analyzerId": true, "responderId": true, "cortexId": true, "cortexJobId": true, "login": true, "assignee": true,
	"owner": true, "createdBy": true, "updatedBy": true, "status": true, "stage": true, "impactStatus": true,
	"dataType": true, "objectType": true, "dataTypeList": true, "cortexIds": true, "severityLabel": true,
	"tlpLabel": true, "papLabel": true, "userPermissions": true, "tactic": true, "tacticLabel": true, "tactics": true,
	"patternType": true, "patternName": true, "hashes": true, "operation": true, "entityType": true,
	"templateId": true, "caseIds": true, "commentId": true, "entityId": true, "entityIds": true, "jobId": true,
	"actionId": true, "targetId": true, "pendingJobIds": true,
	// The date fields, which the wrapper rewrites to a formatted date rather than wraps.
	"date": true, "startDate": true, "endDate": true, "sightedAt": true, "dueDate": true, "occurDate": true,
	"lastSyncDate": true, "_createdAt": true, "_updatedAt": true, "newDate": true, "inProgressDate": true,
	"closedDate": true, "importedDate": true, "alertDate": true, "alertNewDate": true, "alertInProgressDate": true,
	"alertImportedDate": true, "createdAt": true, "updatedAt": true, "lastSuccessDate": true, "lastErrorDate": true,
	"validFrom": true, "expiresAt": true, "includeInTimeline": true,
}

// TrustedFields are the JSON names the analyzer trusts, sorted: the test in internal/utils compares
// them with the wrapper's.
func TrustedFields() []string {
	names := make([]string, 0, len(trusted))
	for name := range trusted {
		names = append(names, name)
	}

	slices.Sort(names)

	return names
}

// Untrusted reports entity free text passed to the strings the server writes itself: the
// TrustedString conversion, Trustedf, the tool errors and their hints. Those skip the
// [UNTRUSTED_DATA] wrapper, so a title there could carry instructions to the model unmarked. The
// text is followed through local variables (taint), not through calls.
var Untrusted = &analysis.Analyzer{
	Name:     "untrusted",
	Doc:      "entity free text never reaches TrustedString, Trustedf, NewToolError(f) or Hint(f) (UNT-001, UNT-002)",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      runUntrusted,
}

func runUntrusted(pass *analysis.Pass) (any, error) {
	insp, _ := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	text := tainted(pass)

	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call, _ := n.(*ast.CallExpr)

		sink := sinkOf(pass, call)
		if sink == "" {
			return
		}

		for _, arg := range call.Args {
			if field := text.text(pass, arg); field != "" {
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

// jsonName is the JSON name of a client field or getter (Title, GetTitle, GetTitleOk): the json tag
// of the struct field it reads, else the generator's convention (UnderscoreId is _id, Title is
// title).
func jsonName(named *types.Named, selector string) string {
	goName := strings.TrimSuffix(strings.TrimPrefix(selector, "Get"), "Ok")
	if goName == "" {
		return ""
	}

	if st, ok := named.Underlying().(*types.Struct); ok {
		for i := range st.NumFields() {
			if st.Field(i).Name() != goName {
				continue
			}

			if tag, _, _ := strings.Cut(reflect.StructTag(st.Tag(i)).Get("json"), ","); tag != "" && tag != "-" {
				return tag
			}
		}
	}

	if rest, ok := strings.CutPrefix(goName, "Underscore"); ok && rest != "" {
		return "_" + lowerFirst(rest)
	}

	return lowerFirst(goName)
}

func lowerFirst(s string) string {
	r := []rune(s)
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

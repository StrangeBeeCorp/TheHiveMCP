package handler

import (
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

// Every field that can hold text is entity text unless the wrapper trusts its JSON name: the
// analyzer denies by default, as the wrapper does.
func byDefault(a *thehive.OutputAlert) []any {
	all, _ := a.ToMap()

	return []any{
		tools.NewToolError("no alert").Hintf("from %s", a.GetSourceRef()), // want `OutputAlert.GetSourceRef reaches ToolError.Hintf`
		utils.Trustedf("a %s alert", a.Type),                              // want `OutputAlert.Type reaches utils.Trustedf`
		utils.Trustedf("alert %v", all),                                   // want `OutputAlert.ToMap \(through all\) reaches utils.Trustedf`
		utils.Trustedf("alert %s is %s, severity %d", a.UnderscoreId, a.GetStatus(), a.GetSeverity()),
	}
}

// A result row, read by a key the wrapper does not trust or by a computed one, is entity text.
func rows(row map[string]any, key string) []any {
	return []any{
		tools.NewToolErrorf("case %v created", row["title"]), // want `row\["title"\] reaches tools.NewToolErrorf`
		utils.Trustedf("%v", row[key]),                       // want `a row read by a computed key reaches utils.Trustedf`
		utils.Trustedf("case %v is %v", row["_id"], row["status"]),
	}
}

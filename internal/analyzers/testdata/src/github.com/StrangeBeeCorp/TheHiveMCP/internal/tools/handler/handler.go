package handler

import (
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

func results(a *thehive.OutputAlert) []any {
	return []any{
		utils.Trustedf("alert %s is %s", a.GetUnderscoreId(), a.GetStatus()),
		utils.Trustedf("created %s", a.GetTitle()), // want `OutputAlert.GetTitle reaches utils.Trustedf`
		utils.TrustedString(a.Title),               // want `OutputAlert.Title reaches utils.TrustedString`
		utils.TrustedString("a fixed sentence"),
	}
}

func failures(a *thehive.OutputAlert) []*tools.ToolError {
	title, _ := a.GetTitleOk()

	return []*tools.ToolError{
		tools.NewToolError("TheHive created no alert").Hint("set entity-data.title"),
		tools.NewToolErrorf("alert %s not found", a.GetUnderscoreId()),
		tools.NewToolErrorf("alert %q not found", a.GetTitle()), // want `OutputAlert.GetTitle reaches tools.NewToolErrorf`
		tools.NewToolError("no alert").Hintf("try %s", a.Title), // want `OutputAlert.Title reaches ToolError.Hintf`
		tools.NewToolError("no alert").Hint(*title),
	}
}

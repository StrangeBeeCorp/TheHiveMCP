package handler

import (
	"fmt"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

// Entity text reaches a sink through local variables, however many.
func flows(a *thehive.OutputAlert) []any {
	title, ok := a.GetTitleOk()
	msg := fmt.Sprintf("alert %s", a.GetTitle())
	copied := msg
	var label string = a.Title
	id := a.GetUnderscoreId()
	n := len(a.Title)

	out := []any{
		tools.NewToolError("no alert").Hint(*title),   // want `OutputAlert.GetTitleOk \(through title\) reaches ToolError.Hint`
		utils.TrustedString(copied),                   // want `OutputAlert.GetTitle \(through copied\) reaches utils.TrustedString`
		tools.NewToolErrorf("%s is gone", label),      // want `OutputAlert.Title \(through label\) reaches tools.NewToolErrorf`
		utils.Trustedf("alert %s: %v, %d", id, ok, n), // an id, a bool and a length carry no text
		tools.NewToolError("no alert").Hintf("retry %s", id),
	}

	for _, tag := range a.Tags {
		out = append(out, utils.Trustedf("tag %s", tag)) // want `OutputAlert.Tags \(through tag\) reaches utils.Trustedf`
	}

	return out
}

// What it does not follow: text a call returns, and text kept in a struct field.
func unfollowed(a *thehive.OutputAlert) []any {
	describe := func() string { return a.Title }
	held := struct{ text string }{text: a.Title}

	return []any{utils.TrustedString(describe()), utils.TrustedString(held.text)}
}

// A variable of the same name in another function is another variable.
func elsewhere() utils.TrustedString {
	title := "a fixed title"

	return utils.TrustedString(title)
}

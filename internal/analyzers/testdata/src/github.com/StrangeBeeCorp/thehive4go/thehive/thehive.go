// Package thehive is a stand-in for the generated client, for the analyzer tests.
package thehive

type OutputAlert struct {
	Title  string
	Status string
}

func (a *OutputAlert) GetTitle() string            { return a.Title }
func (a *OutputAlert) GetTitleOk() (*string, bool) { return &a.Title, true }
func (a *OutputAlert) GetStatus() string           { return a.Status }
func (a *OutputAlert) GetUnderscoreId() string     { return "~1" }

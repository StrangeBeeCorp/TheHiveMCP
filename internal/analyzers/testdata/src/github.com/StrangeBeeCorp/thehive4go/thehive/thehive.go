// Package thehive is a stand-in for the generated client, for the analyzer tests. Its fields, tags
// and methods have the real client's types: gosec type-checks this testdata against the module.
package thehive

type OutputAlert struct {
	UnderscoreId string   `json:"_id"`
	Type         string   `json:"type"`
	SourceRef    string   `json:"sourceRef"`
	Title        string   `json:"title"`
	Severity     int32    `json:"severity"`
	Tags         []string `json:"tags,omitempty"`
	Status       string   `json:"status"`
}

func (a *OutputAlert) GetTitle() string                      { return a.Title }
func (a *OutputAlert) GetTitleOk() (*string, bool)           { return &a.Title, true }
func (a *OutputAlert) GetStatus() string                     { return a.Status }
func (a *OutputAlert) GetUnderscoreId() string               { return a.UnderscoreId }
func (a *OutputAlert) GetSourceRef() string                  { return a.SourceRef }
func (a *OutputAlert) GetSeverity() int32                    { return a.Severity }
func (a OutputAlert) ToMap() (map[string]interface{}, error) { return nil, nil }

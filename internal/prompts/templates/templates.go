package templates

import "embed"

//go:embed *.tmpl
var templatesFS embed.FS

// GetTemplate returns the template content as a string
func GetTemplate(templateName string) (string, error) {
	content, err := templatesFS.ReadFile(templateName)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

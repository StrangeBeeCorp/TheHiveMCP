// Package tools is a stand-in for internal/tools, for the analyzer tests.
package tools

type ToolError struct{ hints []string }

func NewToolError(message string) *ToolError                     { return &ToolError{} }
func NewToolErrorf(format string, args ...any) *ToolError        { return &ToolError{} }
func (e *ToolError) Hint(hint string) *ToolError                 { return e }
func (e *ToolError) Hintf(format string, args ...any) *ToolError { return e }

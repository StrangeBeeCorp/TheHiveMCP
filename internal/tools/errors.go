package tools

import (
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// Error represents a tool error with optional context
type ToolError struct {
	message     string
	cause       error
	hints       []string
	apiResponse interface{}
}

// New creates a new tool error
func NewToolError(message string) *ToolError {
	return &ToolError{message: message}
}

// Newf creates a new tool error with formatting
func NewToolErrorf(format string, args ...interface{}) *ToolError {
	return &ToolError{message: fmt.Sprintf(format, args...)}
}

// Cause adds the underlying error
func (e *ToolError) Cause(err error) *ToolError {
	e.cause = err
	return e
}

// Hint adds a hint message
func (e *ToolError) Hint(hint string) *ToolError {
	e.hints = append(e.hints, hint)
	return e
}

// Hintf adds a formatted hint message
func (e *ToolError) Hintf(format string, args ...interface{}) *ToolError {
	e.hints = append(e.hints, fmt.Sprintf(format, args...))
	return e
}

// Schema adds a hint pointing to the schema resource
func (e *ToolError) Schema(entityType, operation string) *ToolError {
	if operation != "" {
		return e.Hintf("Use get-resource 'hive://schema/%s/%s' for field definitions", entityType, operation)
	}
	return e.Hintf("Use get-resource 'hive://schema/%s' for available fields", entityType)
}

// API adds the API response for debugging
func (e *ToolError) API(resp interface{}) *ToolError {
	e.apiResponse = resp
	return e
}

// Error implements the error interface
func (e *ToolError) Error() string {
	var b strings.Builder
	b.WriteString(e.message)

	if e.cause != nil {
		b.WriteString(": ")
		b.WriteString(e.cause.Error())
	}

	for _, hint := range e.hints {
		b.WriteString(". ")
		b.WriteString(hint)
	}

	if e.apiResponse != nil {
		fmt.Fprintf(&b, ". API response: %v", e.apiResponse)
	}

	return b.String()
}

// Result returns the MCP tool result tuple for direct use in handlers
func (e *ToolError) Result() (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError(e.Error()), nil
}

// Unwrap returns the underlying cause for errors.Is/As support
func (e *ToolError) Unwrap() error {
	return e.cause
}

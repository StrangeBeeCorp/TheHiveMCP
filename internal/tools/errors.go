// Package tools defines the MCP tools exposed by the server and their shared
// registration, validation, and error-handling helpers.
package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

// ToolError represents a tool error with optional context.
type ToolError struct {
	message     string
	cause       error
	hints       []string
	apiResponse any
}

// NewToolError creates a new tool error.
func NewToolError(message string) *ToolError {
	return &ToolError{message: message}
}

// NewToolErrorf creates a new tool error with formatting.
func NewToolErrorf(format string, args ...any) *ToolError {
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
func (e *ToolError) Hintf(format string, args ...any) *ToolError {
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

// API attaches resp's body to the error; consumes and closes it.
func (e *ToolError) API(resp *http.Response) *ToolError {
	if resp == nil {
		return e.Hintf("API response is nil")
	}

	if resp.Body == nil {
		return e.Hintf("API response body is nil")
	}

	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if err != nil {
		return e.Hintf("failed to read API response body: %v", err)
	}

	var jsonObj any

	err = json.Unmarshal(body, &jsonObj)
	if err == nil {
		e.apiResponse = jsonObj
	} else {
		e.apiResponse = string(body)
	}

	return e
}

// Error implements the error interface
func (e *ToolError) Error() string {
	errorObj := map[string]any{
		"error":   true,
		"message": e.message,
	}

	if e.cause != nil {
		errorObj["cause"] = e.cause.Error()
	}

	if len(e.hints) > 0 {
		errorObj["hints"] = e.hints
	}

	if e.apiResponse != nil {
		errorObj["apiResponse"] = e.apiResponse
	}

	errorJSON, err := json.Marshal(errorObj)
	if err != nil {
		return "error: " + e.message
	}

	return string(errorJSON)
}

// ToMap returns the error as a structured map for JSON serialization. These
// maps are embedded in successful bulk results and pass through the
// [UNTRUSTED_DATA] wrapping middleware: message and hints are MCP-authored
// recovery guidance (static text plus agent-supplied inputs) and stay
// unwrapped, while cause and apiResponse carry upstream content and must stay
// plain so they are wrapped (DL-6703).
func (e *ToolError) ToMap() map[string]any {
	errorObj := map[string]any{
		"error":   true,
		"message": utils.TrustedString(e.message),
	}

	if e.cause != nil {
		errorObj["cause"] = e.cause.Error()
	}

	if len(e.hints) > 0 {
		hints := make([]utils.TrustedString, len(e.hints))
		for i, hint := range e.hints {
			hints[i] = utils.TrustedString(hint)
		}

		errorObj["hints"] = hints
	}

	if e.apiResponse != nil {
		errorObj["apiResponse"] = e.apiResponse
	}

	return errorObj
}

// Unwrap returns the underlying cause for errors.Is/As support
func (e *ToolError) Unwrap() error {
	return e.cause
}

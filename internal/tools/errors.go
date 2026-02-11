package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
// Note: This method consumes and closes the response body
func (e *ToolError) API(resp *http.Response) *ToolError {
	// Guard against nil response
	if resp == nil {
		return e.Hintf("API response is nil")
	}

	// Guard against nil body
	if resp.Body == nil {
		return e.Hintf("API response body is nil")
	}

	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close() // Close the body, ignore close errors as they're not critical
	if err != nil {
		return e.Hintf("failed to read API response body: %v", err)
	}

	// Try to parse as JSON first, fallback to string
	var jsonObj interface{}
	if err := json.Unmarshal(body, &jsonObj); err == nil {
		e.apiResponse = jsonObj
	} else {
		// If not valid JSON, store as string
		e.apiResponse = string(body)
	}
	return e
}

// Error implements the error interface
func (e *ToolError) Error() string {
	errorObj := map[string]interface{}{
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

	errorJson, err := json.Marshal(errorObj)
	if err != nil {
		// Fallback to simple string if JSON marshaling fails
		return fmt.Sprintf("error: %s", e.message)
	}

	return string(errorJson)
}

// ToMap returns the error as a structured map for JSON serialization
func (e *ToolError) ToMap() map[string]interface{} {
	errorObj := map[string]interface{}{
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

	return errorObj
}

// Unwrap returns the underlying cause for errors.Is/As support
func (e *ToolError) Unwrap() error {
	return e.cause
}

package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type ToolError struct {
	message     string
	cause       error
	hints       []string
	apiResponse interface{}
}

func NewToolError(message string) *ToolError {
	return &ToolError{message: message}
}

func NewToolErrorf(format string, args ...interface{}) *ToolError {
	return &ToolError{message: fmt.Sprintf(format, args...)}
}

func (e *ToolError) Cause(err error) *ToolError {
	e.cause = err
	return e
}

func (e *ToolError) Hint(hint string) *ToolError {
	e.hints = append(e.hints, hint)
	return e
}

func (e *ToolError) Hintf(format string, args ...interface{}) *ToolError {
	e.hints = append(e.hints, fmt.Sprintf(format, args...))
	return e
}

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

	var jsonObj interface{}
	if err := json.Unmarshal(body, &jsonObj); err == nil {
		e.apiResponse = jsonObj
	} else {
		e.apiResponse = string(body)
	}
	return e
}

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
		return fmt.Sprintf("error: %s", e.message)
	}

	return string(errorJson)
}

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

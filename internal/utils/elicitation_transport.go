package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ElicitationTransport wraps an http.RoundTripper to add elicitation for modifying operations
type ElicitationTransport struct {
	Transport http.RoundTripper
}

// RoundTrip implements http.RoundTripper interface with elicitation for POST, PATCH, DELETE
func (e *ElicitationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	slog.Debug("ElicitationTransport RoundTrip called",
		slog.String("method", req.Method),
		slog.String("url", req.URL.String()))
	// Check if this is a modifying operation that needs confirmation
	if e.requiresElicitation(req.Method, req.URL.Path) {
		slog.Debug("Request requires elicitation",
			slog.String("method", req.Method),
			slog.String("url", req.URL.String()))
		if !e.clientSupportsElicitation(req.Context()) {
			slog.Warn("Client does not support elicitation, allowing request by default",
				slog.String("method", req.Method),
				slog.String("url", req.URL.String()))
			// Allow the request to proceed
			return e.transport().RoundTrip(req)
		}
		if err := e.handleElicitation(req); err != nil {
			return nil, err
		}
	}

	// Execute the request using the underlying transport
	return e.transport().RoundTrip(req)
}

func (e *ElicitationTransport) transport() http.RoundTripper {
	if e.Transport != nil {
		return e.Transport
	}
	return http.DefaultTransport
}

// requiresElicitation determines if the HTTP method requires user confirmation
func (e *ElicitationTransport) requiresElicitation(method string, endpoint string) bool {
	switch strings.ToUpper(method) {
	case "POST", "PATCH", "DELETE":
		if strings.HasSuffix(endpoint, "/api/v1/query") { // Allow queries without elicitation
			return false
		}
		return true
	default:
		return false
	}
}

func (e *ElicitationTransport) clientSupportsElicitation(ctx context.Context) bool {
	session := server.ClientSessionFromContext(ctx)
	sessionWithInfo, ok := session.(server.SessionWithClientInfo)
	if !ok {
		slog.Warn("Client session does not support client info interface")
		return false
	}
	clientCaps := sessionWithInfo.GetClientCapabilities()
	return clientCaps.Elicitation != nil
}

// handleElicitation performs the elicitation request for user confirmation
func (e *ElicitationTransport) handleElicitation(req *http.Request) error {
	ctx := req.Context()

	// Get MCP server from context
	mcpServer := server.ServerFromContext(ctx)

	// Read request body for display (if any)
	var bodyBytes []byte
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return fmt.Errorf("failed to read request body: %w", err)
		}
		// Restore the request body
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	// Format the request details for user confirmation
	requestDetails := e.formatRequestDetails(req, bodyBytes)

	// Create elicitation request
	elicitationRequest := mcp.ElicitationRequest{
		Params: mcp.ElicitationParams{
			Message: fmt.Sprintf("Confirm %s request to TheHive API?\n\n%s", req.Method, requestDetails),
			RequestedSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"confirm": map[string]interface{}{
						"type":        "boolean",
						"description": fmt.Sprintf("Confirm execution of %s request", req.Method),
					},
				},
				"required": []string{"confirm"},
			},
		},
	}

	slog.Info("Requesting elicitation from user",
		slog.String("method", req.Method),
		slog.String("url", req.URL.String()))
	// Request elicitation from client
	result, err := mcpServer.RequestElicitation(ctx, elicitationRequest)

	if err != nil {
		// Check if client doesn't support elicitation
		if errors.Is(err, server.ErrElicitationNotSupported) {
			slog.Warn("Client does not support elicitation, allowing request by default",
				slog.String("method", req.Method),
				slog.String("url", req.URL.String()))
			return nil // Allow the request to proceed
		}

		// Other elicitation errors
		return fmt.Errorf("elicitation request failed: %w for request details: %s", err, requestDetails)
	}

	// Process the elicitation response
	switch result.Action {
	case mcp.ElicitationResponseActionAccept:
		slog.Info("User confirmed request",
			slog.String("method", req.Method),
			slog.String("url", req.URL.String()))
		return nil // Allow the request to proceed

	case mcp.ElicitationResponseActionDecline, mcp.ElicitationResponseActionCancel:
		slog.Info("User declined request",
			slog.String("method", req.Method),
			slog.String("url", req.URL.String()))
		return fmt.Errorf("request declined by user")

	default:
		return fmt.Errorf("unexpected elicitation response action: %s", result.Action)
	}
}

// formatRequestDetails creates a human-readable summary of the request
func (e *ElicitationTransport) formatRequestDetails(req *http.Request, bodyBytes []byte) string {
	var details strings.Builder

	details.WriteString(fmt.Sprintf("Method: %s\n", req.Method))
	details.WriteString(fmt.Sprintf("URL: %s\n", req.URL.String()))

	if len(bodyBytes) > 0 {
		details.WriteString("Payload:\n")

		// Try to pretty-print JSON payload
		if e.isJSONContent(req) {
			var prettyJSON bytes.Buffer
			if err := json.Indent(&prettyJSON, bodyBytes, "", "  "); err == nil {
				details.WriteString(prettyJSON.String())
			} else {
				// Fallback to raw body if JSON parsing fails
				details.WriteString(string(bodyBytes))
			}
		} else {
			// For non-JSON content, just show the raw body (truncated if too long)
			body := string(bodyBytes)
			if len(body) > 500 {
				body = body[:500] + "... (truncated)"
			}
			details.WriteString(body)
		}
	} else {
		details.WriteString("(No payload)")
	}

	return details.String()
}

// isJSONContent checks if the request content type is JSON
func (e *ElicitationTransport) isJSONContent(req *http.Request) bool {
	contentType := req.Header.Get("Content-Type")
	return strings.Contains(strings.ToLower(contentType), "application/json")
}

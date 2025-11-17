package logging

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func getSessionID(ctx context.Context) string {
	session := server.ClientSessionFromContext(ctx)
	if session != nil {
		return session.SessionID()
	}
	return "N/A"
}

func onRegisterSessionHook(ctx context.Context, session server.ClientSession) {
	slog.Debug("Session registered", "id", session.SessionID())
}

func onUnregisterSessionHook(ctx context.Context, session server.ClientSession) {
	slog.Debug("Session unregistered", "id", session.SessionID())
}

func onErrorHook(ctx context.Context, id any, method mcp.MCPMethod, message any, err error) {
	slog.Error("Error occurred", "id", id, "method", method, "message", message, "error", err, "session_id", getSessionID(ctx))
}

func onBeforeInitializeHook(ctx context.Context, id any, message *mcp.InitializeRequest) {
	serializedMessage, _ := json.MarshalIndent(message, "", "  ")
	slog.Info("Initializing session", "id", id, "message", string(serializedMessage), "session_id", getSessionID(ctx))
}

func onAfterInitializeHook(ctx context.Context, id any, message *mcp.InitializeRequest, result *mcp.InitializeResult) {
	serializedResult, _ := json.MarshalIndent(result, "", "  ")
	slog.Info("Session initialized", "id", id, "result", string(serializedResult), "session_id", getSessionID(ctx))
}

func onBeforeListResourcesHook(ctx context.Context, id any, message *mcp.ListResourcesRequest) {
	slog.Debug("Listing resources", "id", id, "message", message, "session_id", getSessionID(ctx))
}

func onAfterListResourcesHook(ctx context.Context, id any, message *mcp.ListResourcesRequest, result *mcp.ListResourcesResult) {
	slog.Debug("Resources listed", "id", id, "result", result, "session_id", getSessionID(ctx))
}

func onBeforeReadResourceHook(ctx context.Context, id any, message *mcp.ReadResourceRequest) {
	slog.Debug("Reading resource", "id", id, "message", message, "session_id", getSessionID(ctx))
}

func onAfterReadResourceHook(ctx context.Context, id any, message *mcp.ReadResourceRequest, result *mcp.ReadResourceResult) {
	slog.Debug("Resource read", "id", id, "result", result, "session_id", getSessionID(ctx))
}

func onBeforeListPromptsHook(ctx context.Context, id any, message *mcp.ListPromptsRequest) {
	slog.Debug("Listing prompts", "id", id, "message", message, "session_id", getSessionID(ctx))
}

func onAfterListPromptsHook(ctx context.Context, id any, message *mcp.ListPromptsRequest, result *mcp.ListPromptsResult) {
	slog.Debug("Prompts listed", "id", id, "result", result, "session_id", getSessionID(ctx))
}

func onBeforeGetPromptHook(ctx context.Context, id any, message *mcp.GetPromptRequest) {
	slog.Debug("Getting prompt", "id", id, "message", message, "session_id", getSessionID(ctx))
}

func onAfterGetPromptHook(ctx context.Context, id any, message *mcp.GetPromptRequest, result *mcp.GetPromptResult) {
	slog.Debug("Prompt retrieved", "id", id, "result", result, "session_id", getSessionID(ctx))
}

func onBeforeListToolsHook(ctx context.Context, id any, message *mcp.ListToolsRequest) {
	slog.Debug("Listing tools", "id", id, "message", message, "session_id", getSessionID(ctx))
}

func onAfterListToolsHook(ctx context.Context, id any, message *mcp.ListToolsRequest, result *mcp.ListToolsResult) {
	slog.Debug("Tools listed", "id", id, "result", result, "session_id", getSessionID(ctx))
}

func onBeforeCallToolHook(ctx context.Context, id any, message *mcp.CallToolRequest) {
	slog.Info("Calling tool", "id", id, "message", message, "session_id", getSessionID(ctx))
}

func onAfterCallToolHook(ctx context.Context, id any, message *mcp.CallToolRequest, result *mcp.CallToolResult) {
	slog.Info("Tool called", "id", id, "result", result, "is_error", result.IsError, "session_id", getSessionID(ctx))
}

func GetLoggingHooks() *server.Hooks {
	return &server.Hooks{
		OnRegisterSession:     []server.OnRegisterSessionHookFunc{onRegisterSessionHook},
		OnUnregisterSession:   []server.OnUnregisterSessionHookFunc{onUnregisterSessionHook},
		OnError:               []server.OnErrorHookFunc{onErrorHook},
		OnBeforeInitialize:    []server.OnBeforeInitializeFunc{onBeforeInitializeHook},
		OnAfterInitialize:     []server.OnAfterInitializeFunc{onAfterInitializeHook},
		OnBeforeListResources: []server.OnBeforeListResourcesFunc{onBeforeListResourcesHook},
		OnAfterListResources:  []server.OnAfterListResourcesFunc{onAfterListResourcesHook},
		OnBeforeReadResource:  []server.OnBeforeReadResourceFunc{onBeforeReadResourceHook},
		OnAfterReadResource:   []server.OnAfterReadResourceFunc{onAfterReadResourceHook},
		OnBeforeListPrompts:   []server.OnBeforeListPromptsFunc{onBeforeListPromptsHook},
		OnAfterListPrompts:    []server.OnAfterListPromptsFunc{onAfterListPromptsHook},
		OnBeforeGetPrompt:     []server.OnBeforeGetPromptFunc{onBeforeGetPromptHook},
		OnAfterGetPrompt:      []server.OnAfterGetPromptFunc{onAfterGetPromptHook},
		OnBeforeListTools:     []server.OnBeforeListToolsFunc{onBeforeListToolsHook},
		OnAfterListTools:      []server.OnAfterListToolsFunc{onAfterListToolsHook},
		OnBeforeCallTool:      []server.OnBeforeCallToolFunc{onBeforeCallToolHook},
		OnAfterCallTool:       []server.OnAfterCallToolFunc{onAfterCallToolHook},
	}
}

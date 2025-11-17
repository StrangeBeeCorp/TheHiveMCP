package logging

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

func LogInitializeLLMCapabilities(sampling bool, openAI bool, openAIBaseURL *string, openAIModelName *string) {
	slog.Info("Initializing LLM capabilities",
		slog.Bool("sampling", sampling),
		slog.Bool("openAI", openAI),
		slog.String("openAIBaseURL", *openAIBaseURL),
		slog.String("openAIModelName", *openAIModelName),
	)
}

const maxLoggedMessageLength = 2000

func LogOpenAIRequest(ctx context.Context, modelName string, messages []mcp.PromptMessage) context.Context {
	messagesJSON, err := json.MarshalIndent(messages, "", "  ")
	if err != nil {
		slog.Warn("Failed to marshal OpenAI request messages", "error", err)
		messagesJSON = []byte("<failed to marshal messages>")
	}
	truncatedMessages := string(messagesJSON)
	if len(truncatedMessages) > maxLoggedMessageLength {
		truncatedMessages = truncatedMessages[:maxLoggedMessageLength] + "..."
	}
	slog.Info("Sending OpenAI request",
		slog.String("modelName", modelName),
		slog.Any("messages", truncatedMessages),
		slog.Time("startTime", time.Now()),
	)
	return context.WithValue(ctx, types.OpenAIRequestStartTimeCtxKey, time.Now())
}

func LogOpenAIResponse(ctx context.Context, modelName string, responseModel interface{}) {
	responseText, err := json.MarshalIndent(responseModel, "", "  ")
	if err != nil {
		slog.Warn("Failed to marshal OpenAI response model", "error", err)
		responseText = []byte("<failed to marshal response>")
	}
	startTime, ok := ctx.Value(types.OpenAIRequestStartTimeCtxKey).(time.Time)
	if !ok {
		slog.Warn("OpenAI request start time not found in context")
		return
	}
	duration := time.Since(startTime)

	slog.Info("Received OpenAI response",
		slog.String("modelName", modelName),
		slog.String("responseText", string(responseText)),
		slog.Duration("duration", duration),
	)
}

func LogSamplingModelRequest(ctx context.Context, messages []mcp.PromptMessage) context.Context {
	messagesJSON, err := json.MarshalIndent(messages, "", "  ")
	if err != nil {
		slog.Warn("Failed to marshal OpenAI request messages", "error", err)
		messagesJSON = []byte("<failed to marshal messages>")
	}
	truncatedMessages := string(messagesJSON)
	if len(truncatedMessages) > maxLoggedMessageLength {
		truncatedMessages = truncatedMessages[:maxLoggedMessageLength] + "..."
	}
	slog.Info("Sending Sampling Model request",
		slog.Any("messages", truncatedMessages),
		slog.Time("startTime", time.Now()),
	)
	return context.WithValue(ctx, types.SamplingModelRequestStartTimeCtxKey, time.Now())
}

func LogSamplingModelResponse(ctx context.Context, responseModel interface{}) {
	responseText, err := json.MarshalIndent(responseModel, "", "  ")
	if err != nil {
		slog.Warn("Failed to marshal Sampling Model response model", "error", err)
		responseText = []byte("<failed to marshal response>")
	}
	startTime, ok := ctx.Value(types.SamplingModelRequestStartTimeCtxKey).(time.Time)
	if !ok {
		slog.Warn("Sampling model request start time not found in context")
		return
	}
	duration := time.Since(startTime)

	slog.Info("Received Sampling Model response",
		slog.String("responseText", string(responseText)),
		slog.Duration("duration", duration),
	)
}

package utils

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func GetSamplingModelCompletion(ctx context.Context, messages []mcp.PromptMessage, target interface{}) error {
	serverFromCtx := server.ServerFromContext(ctx)
	if serverFromCtx == nil {
		return errors.New("no MCP server found in context")
	}
	// Extract system prompt from first message and remaining messages
	var systemPrompt string
	var remainingMessages []mcp.PromptMessage

	if len(messages) > 0 {
		if textContent, ok := messages[0].Content.(mcp.TextContent); ok {
			systemPrompt = textContent.Text
		}
		remainingMessages = messages[1:]
	} else {
		remainingMessages = messages
	}

	samplingMessages := []mcp.SamplingMessage{}
	for _, msg := range remainingMessages {
		samplingMessages = append(samplingMessages, mcp.SamplingMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	samplingCtxWithTimeout, cancel := context.WithTimeout(ctx, types.DefaultMaxCompletionTime)
	defer cancel()
	samplingRequest := mcp.CreateMessageRequest{
		CreateMessageParams: mcp.CreateMessageParams{
			Messages:     samplingMessages,
			SystemPrompt: systemPrompt,
		},
	}

	var responseText string
	for attempt := 0; attempt < types.DefaultMaxCompletionRetries; attempt++ {
		result, err := serverFromCtx.RequestSampling(samplingCtxWithTimeout, samplingRequest)
		if err != nil {
			slog.Warn("Failed to get sampling model completion", "error", err, "attempt", attempt+1)
			continue
		}

		if textContent, ok := result.Content.(mcp.TextContent); ok {
			responseText = textContent.Text
		} else {
			slog.Warn("Sampling model did not return text content", "content", result.Content, "attempt", attempt+1)
			continue
		}

		err = extractAndUnmarshalJSON(responseText, target)

		if err != nil {
			slog.Warn("Failed to extract and unmarshal JSON", "error", err, "content", responseText)
			if strings.Contains(strings.ToLower(responseText), "error") {
				return fmt.Errorf("model returned error response: %s", responseText)
			}
			// Add the error as a new message before retrying
			remainingMessages = append(remainingMessages, mcp.PromptMessage{
				Role:    mcp.RoleAssistant,
				Content: mcp.NewTextContent(responseText),
			})
			remainingMessages = append(remainingMessages, mcp.PromptMessage{
				Role:    mcp.RoleUser,
				Content: mcp.NewTextContent(fmt.Sprintf("The previous response was invalid. Got error: %s. Please try again.", err.Error())),
			})

			// Rebuild samplingMessages with updated remaining messages
			samplingMessages = []mcp.SamplingMessage{}
			for _, msg := range remainingMessages {
				samplingMessages = append(samplingMessages, mcp.SamplingMessage{
					Role:    msg.Role,
					Content: msg.Content,
				})
			}
			samplingRequest.Messages = samplingMessages
			continue
		}
		slog.Debug("Successfully got sampling model completion", "content", responseText)
		return nil
	}

	return errors.New("failed to get sampling model completion after max retries")
}

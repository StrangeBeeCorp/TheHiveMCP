package utils

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
	openai "github.com/sashabaranov/go-openai"
)

// Global OpenAI wrapper instance
var globalOpenAIWrapper *OpenAIWrapper

type OpenAIWrapper struct {
	Client     *openai.Client
	ModelName  string
	MaxTokens  int
	MaxRetries int
}

func translatePromptMessagesToOpenAI(mcpMessages []mcp.PromptMessage) []openai.ChatCompletionMessage {
	openaiMessages := make([]openai.ChatCompletionMessage, 0, len(mcpMessages))

	for i, msg := range mcpMessages {
		var contentText string
		if textContent, ok := msg.Content.(mcp.TextContent); ok {
			contentText = textContent.Text
		}

		// First message becomes system role
		if i == 0 {
			openaiMessages = append(openaiMessages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleSystem,
				Content: contentText,
			})
			continue
		}

		switch msg.Role {
		case mcp.RoleUser:
			openaiMessages = append(openaiMessages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: contentText,
			})
		case mcp.RoleAssistant:
			openaiMessages = append(openaiMessages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: contentText,
			})
		}
	}

	return openaiMessages
}

// InitOpenai initializes the global OpenAI wrapper with configuration from options
// If OpenAI configuration is missing, it logs warnings but does not return errors
func InitOpenai(options *types.TheHiveMcpDefaultOptions) {
	if options.OpenAIAPIKey == "" {
		slog.Warn("OpenAI API key not provided - OpenAI features will be unavailable")
		globalOpenAIWrapper = nil
		return
	}

	slog.Debug("Initializing OpenAI wrapper", "model", options.OpenAIModel, "max_tokens", options.OpenAIMaxTokens)

	config := openai.DefaultConfig(options.OpenAIAPIKey)
	if options.OpenAIBaseURL != "" {
		config.BaseURL = options.OpenAIBaseURL
		slog.Debug("Using custom base URL", "base_url", options.OpenAIBaseURL)
	}

	client := openai.NewClientWithConfig(config)

	globalOpenAIWrapper = &OpenAIWrapper{
		Client:     client,
		ModelName:  options.OpenAIModel,
		MaxTokens:  options.OpenAIMaxTokens,
		MaxRetries: types.DefaultMaxCompletionRetries,
	}

	slog.Info("OpenAI wrapper initialized successfully")
}

func (w *OpenAIWrapper) getModelCompletion(
	ctx context.Context,
	messages []openai.ChatCompletionMessage,
	target interface{},
) error {
	schemaInstruction, err := getSchemaInstruction(target)
	if err != nil {
		return fmt.Errorf("failed to get schema instruction: %w", err)
	}

	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: schemaInstruction,
	})

	req := openai.ChatCompletionRequest{
		Model:     w.ModelName,
		Messages:  messages,
		MaxTokens: w.MaxTokens,
	}

	for attempt := 0; attempt < w.MaxRetries; attempt++ {
		resp, err := w.Client.CreateChatCompletion(ctx, req)
		if err != nil {
			slog.Warn("Failed to get model completion", "error", err, "attempt", attempt+1)
			continue
		}

		if len(resp.Choices) == 0 {
			slog.Warn("No choices in response", "response", resp, "messages", messages)
			continue
		}

		content := resp.Choices[0].Message.Content
		if content == "" {
			slog.Warn("Empty response from model")
			continue
		}

		slog.Debug("Got model completion", "content", content)

		err = extractAndUnmarshalJSON(content, target)
		if err != nil {
			slog.Warn("Failed to unmarshal response", "error", err, "content", content, "attempt", attempt+1)

			// Check if response contains "error"
			if strings.Contains(strings.ToLower(content), "error") {
				return fmt.Errorf("model returned error response: %s", content)
			}

			// Add the error as a new message before retrying
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: content,
			})
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: fmt.Sprintf("The previous response was invalid. Got error: %s. Please try again.", err.Error()),
			})
			req.Messages = messages
			continue
		}
		slog.Debug("Successfully Unmarshalled response")
		return nil
	}

	return errors.New("failed to get model completion after max retries")
}

// GetOpenaiModelCompletionWithWrapper uses the provided OpenAI wrapper
func GetOpenaiModelCompletionWithWrapper(
	ctx context.Context,
	messages []openai.ChatCompletionMessage,
	target interface{},
	wrapper *OpenAIWrapper,
) error {
	if wrapper == nil {
		return errors.New("OpenAI wrapper is nil")
	}

	return wrapper.getModelCompletion(ctx, messages, target)
}

// GetOpenaiModelCompletion uses the globally initialized OpenAI wrapper (deprecated, use context-based approach)
func GetOpenaiModelCompletion(
	ctx context.Context,
	messages []openai.ChatCompletionMessage,
	target interface{},
) error {
	if globalOpenAIWrapper == nil {
		return errors.New("OpenAI not configured - consider implementing sampling alternative")
	}

	return globalOpenAIWrapper.getModelCompletion(ctx, messages, target)
}

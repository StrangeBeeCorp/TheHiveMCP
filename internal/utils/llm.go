package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/logging"
	"github.com/invopop/jsonschema"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Robust function to extract and unmarshal JSON from a string, even if it's wrapped in markdown code blocks or other text.
func extractAndUnmarshalJSON(content string, target interface{}) error {
	//
	if strings.Contains(content, "```") {
		// Use regex to find content between first two ``` markers
		re := regexp.MustCompile("```(?:json)?\\s*\n((?s).*?)```")
		matches := re.FindStringSubmatch(content)

		const matchIndexWithCaptureGroup = 2 // Full match at [0] + first capture group at [1]
		if len(matches) >= matchIndexWithCaptureGroup {
			content = matches[1] // We only want the content between the first two ``` markers
		}
	}

	startIdx := strings.Index(content, "{")
	endIdx := strings.LastIndex(content, "}")

	if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
		content = content[startIdx : endIdx+1]
	}

	// Trim any remaining whitespace
	content = strings.TrimSpace(content)

	// Unmarshal the extracted content
	return json.Unmarshal([]byte(content), target)
}

func generateSchema(t interface{}) interface{} {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	schema := reflector.Reflect(t)
	return schema
}

func getSchemaInstruction(target interface{}) (string, error) {
	schemaModel := generateSchema(target)
	schemaBytes, err := json.MarshalIndent(schemaModel, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal schema model: %w", err)
	}

	schemaInstruction := fmt.Sprintf(
		"Respond with a JSON object that matches the following schema, without any additional text: %s",
		string(schemaBytes),
	)
	return schemaInstruction, nil
}

func checkSamplingSupport(ctx context.Context) (bool, error) {
	session := server.ClientSessionFromContext(ctx)
	if session == nil {
		return false, fmt.Errorf("no client session found in context")
	}

	if clientSession, ok := session.(server.SessionWithClientInfo); ok {
		clientCapabilities := clientSession.GetClientCapabilities()

		if clientCapabilities.Sampling != nil {
			return true, nil
		}
	}

	return false, nil
}

func GetModelCompletion(ctx context.Context, messages []mcp.PromptMessage, target interface{}) error {
	supportsSampling, err := checkSamplingSupport(ctx)
	slog.Info("Found sampling support", "supportsSampling", supportsSampling)
	if err != nil {
		return fmt.Errorf("failed to check sampling support: %w", err)
	}
	if supportsSampling {
		logging.LogSamplingModelRequest(ctx, messages)
		response := GetSamplingModelCompletion(ctx, messages, target)
		logging.LogSamplingModelResponse(ctx, response)
		return response
	}
	logging.LogOpenAIRequest(ctx, globalOpenAIWrapper.ModelName, messages)
	chatMessages := translatePromptMessagesToOpenAI(messages)
	response := GetOpenaiModelCompletion(ctx, chatMessages, target)
	logging.LogOpenAIResponse(ctx, globalOpenAIWrapper.ModelName, response)
	return response
}

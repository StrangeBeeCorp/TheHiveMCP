package prompts

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/auth"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/resources"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func getEntitySchema(entityType string) (string, error) {
	var content []mcp.ResourceContents
	var err error
	switch entityType {
	case types.EntityTypeCase:
		content, err = resources.GetCaseSchemaHandler()
	case types.EntityTypeAlert:
		content, err = resources.GetAlertSchemaHandler()
	case types.EntityTypeObservable:
		content, err = resources.GetObservableSchemaHandler()
	case types.EntityTypeTask:
		content, err = resources.GetTaskSchemaHandler()
	default:
		return "", fmt.Errorf("unsupported entity type: %s", entityType)
	}
	if err != nil {
		return "", fmt.Errorf("failed to get schema for entity type %s: %w", entityType, err)
	}
	if len(content) == 0 {
		return "", fmt.Errorf("no schema content found for entity type %s", entityType)
	}
	if textContent, ok := content[0].(mcp.TextResourceContents); ok {
		return textContent.Text, nil
	} else {
		return "", fmt.Errorf("resource %sSchema did not return text content", entityType)
	}
}

func getEntityFacts(entityType string) (string, error) {
	var content []mcp.ResourceContents
	var err error
	switch entityType {
	case types.EntityTypeCase:
		content, err = resources.GetCaseFactHandler()
	case types.EntityTypeAlert:
		content, err = resources.GetAlertFactHandler()
	case types.EntityTypeObservable:
		content, err = resources.GetObservableFactHandler()
	case types.EntityTypeTask:
		content, err = resources.GetTaskFactHandler()
	default:
		return "", fmt.Errorf("unsupported entity type: %s", entityType)
	}
	if err != nil {
		return "", fmt.Errorf("failed to get facts for entity type %s: %w", entityType, err)
	}
	if len(content) == 0 {
		return "", fmt.Errorf("no facts content found for entity type %s", entityType)
	}
	if textContent, ok := content[0].(mcp.TextResourceContents); ok {
		return textContent.Text, nil
	} else {
		return "", fmt.Errorf("resource %sFacts did not return text content", entityType)
	}
}

func GetBuildFiltersPrompt(ctx context.Context, userQuery string, entityType string) (*mcp.GetPromptResult, error) {
	entitySchema, err := getEntitySchema(entityType)
	if err != nil {
		return nil, fmt.Errorf("failed to get entity schema: %w", err)
	}
	entityFacts, err := getEntityFacts(entityType)
	if err != nil {
		return nil, fmt.Errorf("failed to get entity facts: %w", err)
	}
	config := PromptConfig{
		TemplateName: "build_filters.tmpl",
		ExampleFile:  "build_filters_examples",
		CustomData:   map[string]interface{}{"EntityType": entityType, "EntitySchema": entitySchema, "EntityFacts": entityFacts},
		UserQuery:    userQuery,
		Title:        "Filters build assistance",
	}
	promptResult, err := ProcessPromptWithExamples(ctx, config)
	slog.Debug("Processed build filters prompt", "result", promptResult)
	if err != nil {
		return nil, fmt.Errorf("failed to process prompt: %w", err)
	}

	return promptResult, nil
}

func RegisterBuildFiltersPromptHandler(s *server.MCPServer) {
	buildFiltersPrompt := mcp.NewPrompt(
		"build-filters",
		mcp.WithPromptDescription("Generate filters to search for entities in the Hive"),
		mcp.WithArgument("query", mcp.ArgumentDescription("The query to search for entities")),
	)

	handler := func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		entityType := request.Params.Arguments["entity-type"]
		userQuery := request.Params.Arguments["query"]
		return GetBuildFiltersPrompt(ctx, userQuery, entityType)
	}

	s.AddPrompt(buildFiltersPrompt, auth.AuthenticatedPromptHandlerFunc(handler))
}

package prompts

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"text/template"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/prompts/templates"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/resources"
	"github.com/mark3labs/mcp-go/mcp"
	"gopkg.in/yaml.v3"
)

// ResourceMapping defines which function to call for each template reference
var SaticResourceMapping = map[string]func() ([]mcp.ResourceContents, error){
	"DateFacts":        resources.GetDateFactHandler,
	"HiveFacts":        resources.GetHiveFactHandler,
	"TaskFacts":        resources.GetTaskFactHandler,
	"ObservableFacts":  resources.GetObservableFactHandler,
	"AlertFacts":       resources.GetAlertFactHandler,
	"CaseFacts":        resources.GetCaseFactHandler,
	"AnalyzerFacts":    resources.GetAnalyzerFactHandler,
	"ResponderFacts":   resources.GetResponderFactHandler,
	"TaskSchema":       resources.GetTaskSchemaHandler,
	"ObservableSchema": resources.GetObservableSchemaHandler,
	"AlertSchema":      resources.GetAlertSchemaHandler,
	"CaseSchema":       resources.GetCaseSchemaHandler,
	"Formatting":       resources.GetFormattingRuleHandler,
	"Integrity":        resources.GetIntegrityRuleHandler,
	"Filtering":        resources.GetFilteringRuleHandler,
	"FilterSchema":     resources.GetFilterSchemaHandler,
}

var DynamicResourceMapping = map[string]func(context.Context, mcp.ReadResourceRequest) ([]mcp.ResourceContents, error){
	"AvailableUsers":           resources.GetAvailableUsers,
	"AvailableCaseTemplates":   resources.GetAvailableCaseTemplates,
	"AvailableCaseStatuses":    resources.GetAvailableCaseStatuses,
	"AvailableObservableTypes": resources.GetAvailableObservableTypes,
	"CurrentUser":              resources.GetCurrentUser,
}

// PromptData contains both system-managed and custom data for template processing
type PromptData struct {
	StaticData  map[string]interface{} // Filled automatically based on StaticResourceMapping
	CustomData  map[string]interface{} // Custom data provided by the caller
	DynamicData map[string]interface{} // Filled automatically based on DynamicResourceMapping
}

type Example struct {
	Data      string `yaml:"data,omitempty"`
	User      string `yaml:"user"`
	Assistant string `yaml:"assistant"`
}

type PromptConfig struct {
	TemplateName string                 // Name of the template file to use
	ExampleFile  string                 // Name of the examples file (without extension), empty for no examples
	CustomData   map[string]interface{} // Additional data for system prompt template
	UserData     string                 // Data to show before user query (optional)
	UserQuery    string                 // The actual user query
	Title        string                 // Title of the prompt
}

// ProcessPrompt processes a prompt template file with both system and custom data
func ProcessPrompt(ctx context.Context, templateName string, customData map[string]interface{}) (string, error) {
	// Read template using embed
	templateContent, err := templates.GetTemplate(templateName)
	if err != nil {
		return "", fmt.Errorf("failed to read template %s: %w", templateName, err)
	}

	// Create template data structure
	data := PromptData{
		StaticData:  make(map[string]interface{}),
		DynamicData: make(map[string]interface{}),
		CustomData:  customData,
	}

	// Fetch all mapped resources
	for resourceName, resourceFunc := range SaticResourceMapping {
		content, err := resourceFunc()
		if err != nil {
			return "", fmt.Errorf("failed to get resource %s: %w", resourceName, err)
		}

		// Extract text content from the first resource
		if len(content) > 0 {
			if textContent, ok := content[0].(mcp.TextResourceContents); ok {
				data.StaticData[resourceName] = textContent.Text
			} else {
				return "", fmt.Errorf("resource %s did not return text content", resourceName)
			}
		} else {
			return "", fmt.Errorf("resource %s returned no content", resourceName)
		}
	}
	// Fetch all mapped resources
	for resourceName, resourceFunc := range DynamicResourceMapping {
		content, err := resourceFunc(ctx, mcp.ReadResourceRequest{})
		if err != nil {
			return "", fmt.Errorf("failed to get resource %s: %w", resourceName, err)
		}
		// Extract text content from the first resource
		if len(content) > 0 {
			if textContent, ok := content[0].(mcp.TextResourceContents); ok {
				data.DynamicData[resourceName] = textContent.Text
			} else {
				return "", fmt.Errorf("resource %s did not return text content", resourceName)
			}
		} else {
			return "", fmt.Errorf("resource %s returned no content", resourceName)
		}
	}

	// Create and process template
	tmpl, err := template.New(templateName).Parse(string(templateContent))
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// ProcessPromptWithExamples processes a prompt with all its components
func ProcessPromptWithExamples(ctx context.Context, config PromptConfig) (*mcp.GetPromptResult, error) {
	// Process system prompt template
	systemPrompt, err := ProcessPrompt(ctx, config.TemplateName, config.CustomData)
	if err != nil {
		return nil, fmt.Errorf("failed to process system prompt: %w", err)
	}

	messages := []mcp.PromptMessage{
		mcp.NewPromptMessage(
			mcp.RoleUser,
			mcp.NewTextContent(systemPrompt),
		),
	}

	// Load and add examples if provided
	if config.ExampleFile != "" {
		exampleMessages, err := loadExamples(config.ExampleFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load examples: %w", err)
		}
		messages = append(messages, exampleMessages...)
	}

	// Add user data if provided
	if config.UserData != "" {
		messages = append(messages, mcp.NewPromptMessage(
			mcp.RoleUser,
			mcp.NewTextContent(config.UserData),
		))
	}

	// Add user query as the final message
	if config.UserQuery != "" {
		messages = append(messages, mcp.NewPromptMessage(
			mcp.RoleUser,
			mcp.NewTextContent(config.UserQuery),
		))
	}

	return mcp.NewGetPromptResult(
		config.Title,
		messages,
	), nil
}

//go:embed examples/*.yaml
var examplesFS embed.FS

// Helper function to load examples from YAML file
func loadExamples(exampleName string) ([]mcp.PromptMessage, error) {
	// Get the directory where prompt_processor.go is located
	filename := fmt.Sprintf("examples/%s.yaml", exampleName)
	data, err := examplesFS.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read examples file: %w", err)
	}

	var examples []Example
	if err := yaml.Unmarshal(data, &examples); err != nil {
		return nil, fmt.Errorf("failed to parse examples: %w", err)
	}
	const messagePerExample = 3 // *3 because we might have data+user+assistant
	messages := make([]mcp.PromptMessage, 0, len(examples)*messagePerExample)
	for _, example := range examples {
		// Add data message if present
		if example.Data != "" {
			messages = append(messages, mcp.NewPromptMessage(
				mcp.RoleUser,
				mcp.NewTextContent(example.Data),
			))
		}
		// Add user query
		messages = append(messages,
			mcp.NewPromptMessage(
				mcp.RoleUser,
				mcp.NewTextContent(example.User),
			),
			mcp.NewPromptMessage(
				mcp.RoleAssistant,
				mcp.NewTextContent(example.Assistant),
			),
		)
	}

	return messages, nil
}

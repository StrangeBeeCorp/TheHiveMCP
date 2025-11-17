package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"
)

type SimplifiedUser struct {
	ID           string `json:"_id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Organisation string `json:"organisation"`
	Profile      string `json:"profile"`
	Type         string `json:"type"`
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Filter out unnecessary user fields
func parseUsers(results interface{}) (string, error) {
	// Convert interface{} -> JSON -> []thehive.OutputUser
	resultBytes, err := json.Marshal(results)
	if err != nil {
		return "", fmt.Errorf("failed to marshal results: %w", err)
	}

	var users []thehive.OutputUser
	if err := json.Unmarshal(resultBytes, &users); err != nil {
		return "", fmt.Errorf("failed to unmarshal to OutputUser: %w", err)
	}

	// Create a simplified slice of users with only relevant fields
	var simplifiedUsers []SimplifiedUser
	for _, user := range users {
		simplifiedUsers = append(simplifiedUsers, SimplifiedUser{
			ID:           user.UnderscoreId,
			Name:         user.Name,
			Email:        derefString(user.Email),
			Organisation: user.Organisation,
			Profile:      user.Profile,
			Type:         user.Type,
		})
	}

	// Marshal the simplified users to JSON
	simplifiedUsersJSON, err := json.MarshalIndent(simplifiedUsers, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal simplified users: %w", err)
	}
	return string(simplifiedUsersJSON), nil
}

func GetAvailableUsers(ctx context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client from context: %w. Check authentication and connection settings", err)
	}
	operation := thehive.NewInputQueryGenericOperation("listUser")
	hiveQuery := thehive.InputQuery{
		Query: []thehive.InputQueryNamedOperation{
			thehive.InputQueryGenericOperationAsInputQueryNamedOperation(operation),
		},
	}
	results, resp, err := hiveClient.QueryAndExportAPI.QueryAPI(ctx).InputQuery(hiveQuery).Execute()

	if err != nil {
		return nil, fmt.Errorf("failed to find users: %w. Check that you have permissions to list users. API response: %v", err, resp)
	}
	usersJSON, err := parseUsers(results)
	if err != nil {
		return nil, fmt.Errorf("failed to parse users: %w", err)
	}
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "hive://metadata/organization/users",
			MIMEType: "application/json",
			Text:     string(usersJSON),
		},
	}, nil
}

func GetAvailableCaseTemplates(ctx context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client from context: %w. Check authentication and connection settings", err)
	}

	operation := thehive.NewInputQueryGenericOperation("listCaseTemplate")
	hiveQuery := thehive.InputQuery{
		Query: []thehive.InputQueryNamedOperation{
			thehive.InputQueryGenericOperationAsInputQueryNamedOperation(operation),
		},
	}
	caseTemplates, resp, err := hiveClient.QueryAndExportAPI.QueryAPI(ctx).InputQuery(hiveQuery).Execute()

	if err != nil {
		return nil, fmt.Errorf("failed to find case templates: %w. Check that you have permissions to list case templates. API response: %v", err, resp)
	}

	caseTemplatesJSON, err := json.MarshalIndent(caseTemplates, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal case templates: %w", err)
	}
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "hive://metadata/entities/case/templates",
			MIMEType: "application/json",
			Text:     string(caseTemplatesJSON),
		},
	}, nil
}

func GetAvailableAnalyzers(ctx context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client from context: %w. Check authentication and connection settings", err)
	}

	analyzers, resp, err := hiveClient.CortexAPI.ListAnalyzers(ctx).Range_("0-100").Execute()

	if err != nil {
		return nil, fmt.Errorf("failed to find analyzers: %w. Check that Cortex integration is enabled and you have permissions to list analyzers. API response: %v", err, resp)
	}

	analyzersJSON, err := json.MarshalIndent(analyzers, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal analyzers: %w", err)
	}
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "hive://metadata/automation/analyzers",
			MIMEType: "application/json",
			Text:     string(analyzersJSON),
		},
	}, nil
}

func GetAvailableResponders(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client from context: %w. Check authentication and connection settings", err)
	}

	// Extract entity type and ID from request URI query parameters
	// Expected format: hive://metadata/automation/responders?entityType=case&entityId=~123456
	parsedURI, err := url.Parse(req.Params.URI)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URI: %w", err)
	}

	query := parsedURI.Query()
	entityType := query.Get("entityType")
	entityID := query.Get("entityId")

	if entityType == "" || entityID == "" {
		return nil, fmt.Errorf("entityType and entityId query parameters are required. Example: hive://metadata/automation/responders?entityType=case&entityId=~123456")
	}

	responders, resp, err := hiveClient.CortexAPI.ListResponders(ctx, entityType, entityID).Execute()

	if err != nil {
		return nil, fmt.Errorf("failed to find responders for %s %s: %w. Check that Cortex integration is enabled and you have permissions to list responders. API response: %v", entityType, entityID, err, resp)
	}

	respondersJSON, err := json.MarshalIndent(responders, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal responders: %w", err)
	}
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      fmt.Sprintf("hive://metadata/automation/responders?entityType=%s&entityId=%s", entityType, entityID),
			MIMEType: "application/json",
			Text:     string(respondersJSON),
		},
	}, nil
}

func GetAvailableCaseStatuses(ctx context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client from context: %w. Check authentication and connection settings", err)
	}

	operation := thehive.NewInputQueryGenericOperation("listCaseStatus")
	hiveQuery := thehive.InputQuery{
		Query: []thehive.InputQueryNamedOperation{
			thehive.InputQueryGenericOperationAsInputQueryNamedOperation(operation),
		},
	}
	caseStatuses, resp, err := hiveClient.QueryAndExportAPI.QueryAPI(ctx).InputQuery(hiveQuery).Execute()

	if err != nil {
		return nil, fmt.Errorf("failed to find case statuses: %w, %v", err, resp)
	}
	caseStatusesJSON, err := json.MarshalIndent(caseStatuses, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal case statuses: %w", err)
	}
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "hive://metadata/entities/case/statuses",
			MIMEType: "application/json",
			Text:     string(caseStatusesJSON),
		},
	}, nil
}

func GetCurrentUser(ctx context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client from context: %w. Check authentication and connection settings", err)
	}

	currentUser, resp, err := hiveClient.UserAPI.GetCurrentUserInfo(ctx).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user information: %w. Check authentication status. API response: %v", err, resp)
	}
	currentUserJSON, err := json.MarshalIndent(currentUser, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal current user: %w", err)
	}
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "hive://config/current-user",
			MIMEType: "application/json",
			Text:     string(currentUserJSON),
		},
	}, nil
}

func GetAvailableObservableTypes(ctx context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client from context: %w. Check authentication and connection settings", err)
	}

	operation := thehive.NewInputQueryGenericOperation("listObservableType")
	hiveQuery := thehive.InputQuery{
		Query: []thehive.InputQueryNamedOperation{
			thehive.InputQueryGenericOperationAsInputQueryNamedOperation(operation),
		},
	}
	observableTypes, resp, err := hiveClient.QueryAndExportAPI.QueryAPI(ctx).InputQuery(hiveQuery).Execute()

	if err != nil {
		return nil, fmt.Errorf("failed to find observable types: %w, %v", err, resp)
	}
	observableTypesJSON, err := json.MarshalIndent(observableTypes, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal observable types: %w", err)
	}
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "hive://metadata/entities/observable/types",
			MIMEType: "application/json",
			Text:     string(observableTypesJSON),
		},
	}, nil
}

func GetAvailableCustomFields(ctx context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client from context: %w. Check authentication and connection settings", err)
	}
	customFields, resp, err := hiveClient.CustomFieldAPI.ListCustomFields(ctx).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to find custom fields: %w, %v", err, resp)
	}
	customFieldsJSON, err := json.MarshalIndent(customFields, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal custom fields: %w", err)
	}
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "hive://metadata/entities/custom-fields",
			MIMEType: "application/json",
			Text:     string(customFieldsJSON),
		},
	}, nil
}

func RegisterDynamicResources(registry *ResourceRegistry) {
	// Register available users
	availableUsers := mcp.NewResource(
		"hive://metadata/organization/users",
		"Users",
		mcp.WithResourceDescription("List of users in the organization for assignment"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(availableUsers, GetAvailableUsers)

	// Register available case templates
	availableCaseTemplates := mcp.NewResource(
		"hive://metadata/entities/case/templates",
		"Case Templates",
		mcp.WithResourceDescription("Available case templates with predefined tasks and fields"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(availableCaseTemplates, GetAvailableCaseTemplates)

	// Register available analyzers
	availableAnalyzers := mcp.NewResource(
		"hive://metadata/automation/analyzers",
		"Analyzers",
		mcp.WithResourceDescription("Available Cortex analyzers for observable enrichment"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(availableAnalyzers, GetAvailableAnalyzers)

	// Register available responders
	availableResponders := mcp.NewResource(
		"hive://metadata/automation/responders",
		"Responders",
		mcp.WithResourceDescription("Available Cortex responders for active response. Requires entityType and entityId query parameters."),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(availableResponders, GetAvailableResponders)

	// Register available case statuses
	availableCaseStatuses := mcp.NewResource(
		"hive://metadata/entities/case/statuses",
		"Case Statuses",
		mcp.WithResourceDescription("Available status values for cases (New, InProgress, Resolved, etc.)"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(availableCaseStatuses, GetAvailableCaseStatuses)

	// Register Current user
	currentUser := mcp.NewResource(
		"hive://config/current-user",
		"Current User",
		mcp.WithResourceDescription("Currently authenticated user information"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(currentUser, GetCurrentUser)

	// Register available observable types
	availableObservableTypes := mcp.NewResource(
		"hive://metadata/entities/observable/types",
		"Observable Types",
		mcp.WithResourceDescription("Available observable data types (ip, domain, hash, url, etc.)"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(availableObservableTypes, GetAvailableObservableTypes)

	// Register available custom fields
	availableCustomFields := mcp.NewResource(
		"hive://metadata/entities/custom-fields",
		"Custom Fields",
		mcp.WithResourceDescription("Organization-defined custom fields across all entities"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(availableCustomFields, GetAvailableCustomFields)
}

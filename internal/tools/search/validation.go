package search

import (
	"context"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

func (t *SearchTool) ValidatePermissions(ctx context.Context, params SearchEntitiesParams) error {
	permissions, err := utils.GetPermissionsFromContext(ctx)
	if err != nil {
		return tools.NewToolError("failed to get permissions").Cause(err)
	}

	if !permissions.IsToolAllowed(t.Name()) {
		return tools.NewToolErrorf("tool %s is not permitted by your permissions configuration", t.Name())
	}

	return nil
}

func (t *SearchTool) ValidateParams(params *SearchEntitiesParams) error {
	// Apply defaults first
	if params.SortBy == "" {
		params.SortBy = "_createdAt"
	}

	if params.SortOrder == "" {
		params.SortOrder = "desc"
	}

	if params.Limit == 0 {
		params.Limit = 10
	}

	if len(params.ExtraColumns) == 0 {
		// Use entity-specific default fields
		if defaultFields, exists := types.DefaultFields[params.EntityType]; exists {
			params.ExtraColumns = defaultFields
		} else {
			params.ExtraColumns = []string{"_id", "title", "url"} // fallback
		}
	}

	if params.ExtraData == nil {
		params.ExtraData = []string{}
	}

	if params.AdditionalQueries == nil {
		params.AdditionalQueries = []string{}
	}

	// Validate entity type
	validEntityTypes := []string{types.EntityTypeAlert, types.EntityTypeCase, types.EntityTypeTask, types.EntityTypeObservable}
	var isValidEntityType bool
	for _, validType := range validEntityTypes {
		if params.EntityType == validType {
			isValidEntityType = true
			break
		}
	}
	if !isValidEntityType {
		return tools.NewToolErrorf("invalid entity-type '%s'. Must be one of: 'alert', 'case', 'task', 'observable'", params.EntityType)
	}

	// Validate query is not empty
	if params.Query == "" {
		return tools.NewToolError("query parameter is required. Provide a natural language description of what entities to find, e.g., 'high severity alerts from last week'")
	}

	// Validate sort order
	if params.SortOrder != "asc" && params.SortOrder != "desc" {
		return tools.NewToolErrorf("invalid sort-order '%s'. Must be 'asc' or 'desc'", params.SortOrder)
	}

	// Validate limit
	if params.Limit < 0 {
		return tools.NewToolError("limit must be a non-negative integer")
	}
	if params.Limit > 1000 {
		return tools.NewToolError("limit cannot exceed 1000 entities")
	}

	return nil
}

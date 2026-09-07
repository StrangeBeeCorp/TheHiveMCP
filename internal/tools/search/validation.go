package search

import (
	"context"
	"slices"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

// ValidatePermissions checks that the caller is allowed to use the search tool.
func (t *Tool) ValidatePermissions(ctx context.Context, _ EntitiesParams) error {
	permissions, err := utils.GetPermissionsFromContext(ctx)
	if err != nil {
		return tools.NewToolError("failed to get permissions").Cause(err)
	}

	if !permissions.IsToolAllowed(t.Name()) {
		return tools.NewToolErrorf("tool %s is not permitted by your permissions configuration", t.Name())
	}

	return nil
}

// ValidateParams applies defaults and validates the search parameters in place.
func (t *Tool) ValidateParams(params *EntitiesParams) error {
	if params.SortBy == "" {
		params.SortBy = types.DefaultSortField(params.EntityType)
	}

	if params.SortOrder == "" {
		params.SortOrder = SortOrderDesc
	}

	if params.Limit == 0 {
		params.Limit = DefaultSearchLimit
	}

	if len(params.ExtraColumns) == 0 {
		if defaultFields, exists := types.DefaultFields[params.EntityType]; exists {
			params.ExtraColumns = defaultFields
		} else {
			params.ExtraColumns = []string{fieldID, fieldTitle, "url"} // fallback
		}
	}

	if params.ExtraData == nil {
		params.ExtraData = []string{}
	}

	if params.AdditionalQueries == nil {
		params.AdditionalQueries = []string{}
	}

	if !slices.Contains(ValidEntityTypes, params.EntityType) {
		return tools.NewToolErrorf("invalid entity-type '%s'. Must be one of: '%s'", params.EntityType, strings.Join(ValidEntityTypes, "', '"))
	}

	// Filters are optional (empty = match-all) and validated by TheHive at query time, not here.

	if params.SortOrder != SortOrderAsc && params.SortOrder != SortOrderDesc {
		return tools.NewToolErrorf("invalid sort-order '%s'. Must be 'asc' or 'desc'", params.SortOrder)
	}

	if params.Limit < 0 {
		return tools.NewToolError("limit must be a non-negative integer")
	}

	if params.Limit > 1000 {
		return tools.NewToolError("limit cannot exceed 1000 entities")
	}

	if params.Offset < 0 {
		return tools.NewToolError("offset must be a non-negative integer")
	}

	// The index bounds the WINDOW (first row + rows read), not the offset alone,
	// and the truncation probe makes the window one row wider than the limit.
	// Written as a subtraction rather than Offset+Limit+1 > MaxSearchWindow so a
	// caller sending a near-maxint offset cannot overflow past the check; Limit
	// is already bounded above, so the right-hand side cannot go negative.
	if params.Offset > MaxSearchWindow-params.Limit-1 {
		return tools.NewToolErrorf(
			"offset %d with limit %d reads past the maximum result window of %d rows. Narrow the filter instead of paging further, or use count=true for the size of the match",
			params.Offset, params.Limit, MaxSearchWindow)
	}

	return nil
}

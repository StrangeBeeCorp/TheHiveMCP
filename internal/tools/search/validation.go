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
	applyEntitiesDefaults(params)

	if !slices.Contains(ValidEntityTypes, params.EntityType) {
		return tools.NewToolErrorf("invalid entity-type '%s'. Must be one of: '%s'", params.EntityType, strings.Join(ValidEntityTypes, "', '"))
	}

	// Filters are optional (empty = match-all) and validated by TheHive at query time, not here.

	if params.SortOrder != SortOrderAsc && params.SortOrder != SortOrderDesc {
		return tools.NewToolErrorf("invalid sort-order '%s'. Must be 'asc' or 'desc'", params.SortOrder)
	}

	return validatePagingWindow(params)
}

// applyEntitiesDefaults fills in every parameter the caller left unset, so the
// validation below and the handler both see a fully-populated request.
func applyEntitiesDefaults(params *EntitiesParams) {
	if params.SortBy == "" {
		params.SortBy = types.DefaultSortField(params.EntityType)
	}

	if params.SortOrder == "" {
		params.SortOrder = SortOrderDesc
	}

	if params.Limit == 0 {
		params.Limit = DefaultSearchLimit
	}

	// Set, extra-columns REPLACES the defaults — it is a projection, not an
	// addition, which is what lets a caller trim a wide entity down to the few
	// fields it needs. TestExtraColumnsLimitColumns pins that. The name reads
	// additive and has misled at least one caller, so the parameter description
	// spells the replacement out.
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
}

// validatePagingWindow bounds the slice of the result set a search may read.
//
// Defaults are already applied, so Limit is at least 1 here.
func validatePagingWindow(params *EntitiesParams) error {
	if params.Limit < 0 {
		return tools.NewToolError("limit must be a non-negative integer")
	}

	if params.Limit > MaxSearchLimit {
		return tools.NewToolErrorf("limit cannot exceed %d entities", MaxSearchLimit)
	}

	if params.Offset < 0 {
		return tools.NewToolError("offset must be a non-negative integer")
	}

	// A count reads no window: TheHive returns a bare number, and the handler
	// skips paging entirely. Bounding it here rejected count=true&offset=9995
	// with a message telling the caller to use count=true.
	if params.Count {
		return nil
	}

	// The index bounds the WINDOW (first row + rows read), not the offset alone.
	// The truncation probe would make that window one row wider than the limit,
	// but buildPagingOperation clamps it to MaxSearchWindow instead of pushing
	// past it, so the bound here is the page itself: a page ending exactly on
	// the last readable row is legal, and only loses the ability to report
	// hasMore — there is nothing further to report.
	//
	// Written as a subtraction rather than Offset+Limit > MaxSearchWindow so a
	// caller sending a near-maxint offset cannot overflow past the check; Limit
	// is already bounded above, so the right-hand side cannot go negative.
	if params.Offset > MaxSearchWindow-params.Limit {
		return tools.NewToolErrorf(
			"offset %d with limit %d reads past the maximum result window of %d rows. Narrow the filter instead of paging further, or use count=true for the size of the match",
			params.Offset, params.Limit, MaxSearchWindow)
	}

	return nil
}

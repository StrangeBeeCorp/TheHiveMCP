package permissions

import (
	"fmt"
)

// MergeFilters combines permission filters with user-provided query filters
// Returns the merged filter and a boolean indicating if permission filters were applied
func MergeFilters(userQuery map[string]interface{}, permissionFilters map[string]interface{}) (map[string]interface{}, bool) {
	// If no permission filters, return original query
	if len(permissionFilters) == 0 {
		return userQuery, false
	}

	// If no user query, return permission filters only
	if len(userQuery) == 0 {
		return permissionFilters, true
	}

	// Both exist - merge with AND logic
	merged := map[string]interface{}{
		"_and": []interface{}{
			userQuery,
			permissionFilters,
		},
	}

	return merged, true
}

// ApplyFiltersToQuery applies permission filters to a TheHive query
// This is used by search and manage tools to ensure queries respect permissions
func ApplyFiltersToQuery(query map[string]interface{}, permFilters map[string]interface{}) (map[string]interface{}, bool, error) {
	if len(permFilters) == 0 {
		return query, false, nil
	}

	// Extract the existing query filter if present
	var existingFilter map[string]interface{}
	if query != nil {
		if filter, ok := query["query"]; ok {
			if filterMap, ok := filter.(map[string]interface{}); ok {
				existingFilter = filterMap
			}
		}
	}

	// Merge filters
	mergedFilter, applied := MergeFilters(existingFilter, permFilters)
	if !applied {
		return query, false, nil
	}

	// Create new query with merged filter
	if query == nil {
		query = make(map[string]interface{})
	}

	// Make a copy to avoid modifying the original
	resultQuery := make(map[string]interface{})
	for k, v := range query {
		resultQuery[k] = v
	}

	// Apply the merged filter
	resultQuery["query"] = mergedFilter

	return resultQuery, true, nil
}

// ValidateFilter performs basic validation on a filter structure
func ValidateFilter(filter map[string]interface{}) error {
	if len(filter) == 0 {
		return nil
	}

	// Check for valid TheHive filter operators
	validOperators := map[string]bool{
		"_and":   true,
		"_or":    true,
		"_not":   true,
		"_field": true,
	}

	for key := range filter {
		if key == "_field" || key == "_operator" || key == "_value" {
			continue
		}
		if !validOperators[key] {
			// Could be a custom field, which is valid
			continue
		}
	}

	return nil
}

// PermissionInfo describes how permissions affected a response
type PermissionInfo struct {
	Applied       bool     `json:"applied"`
	FilterApplied bool     `json:"filter_applied,omitempty"`
	Message       string   `json:"message,omitempty"`
	Restrictions  []string `json:"restrictions,omitempty"`
}

// NewPermissionInfo creates a PermissionInfo with applied=false
func NewPermissionInfo() PermissionInfo {
	return PermissionInfo{Applied: false}
}

// NewPermissionInfoDenied creates a PermissionInfo for a denied operation
func NewPermissionInfoDenied(message string) PermissionInfo {
	return PermissionInfo{
		Applied: true,
		Message: message,
	}
}

// NewPermissionInfoFiltered creates a PermissionInfo for a filtered operation
func NewPermissionInfoFiltered(message string) PermissionInfo {
	return PermissionInfo{
		Applied:       true,
		FilterApplied: true,
		Message:       message,
	}
}

// NewPermissionInfoRestricted creates a PermissionInfo with restrictions list
func NewPermissionInfoRestricted(restrictions []string) PermissionInfo {
	return PermissionInfo{
		Applied:      true,
		Restrictions: restrictions,
		Message:      fmt.Sprintf("%d items restricted by permissions", len(restrictions)),
	}
}

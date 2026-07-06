package permissions

import (
	"fmt"
)

// MergeFilters ANDs permission filters into the user query; the bool reports whether any were applied.
func MergeFilters(userQuery, permissionFilters map[string]any) (map[string]any, bool) {
	if len(permissionFilters) == 0 {
		return userQuery, false
	}

	if len(userQuery) == 0 {
		return permissionFilters, true
	}

	merged := map[string]any{
		"_and": []any{
			userQuery,
			permissionFilters,
		},
	}

	return merged, true
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

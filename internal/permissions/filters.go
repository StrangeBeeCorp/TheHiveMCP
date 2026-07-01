package permissions

import (
	"fmt"
)

// MergeFilters ANDs permission filters into the user query; the bool reports whether any were applied.
func MergeFilters(userQuery map[string]interface{}, permissionFilters map[string]interface{}) (map[string]interface{}, bool) {
	if len(permissionFilters) == 0 {
		return userQuery, false
	}

	if len(userQuery) == 0 {
		return permissionFilters, true
	}

	merged := map[string]interface{}{
		"_and": []interface{}{
			userQuery,
			permissionFilters,
		},
	}

	return merged, true
}

type PermissionInfo struct {
	Applied       bool     `json:"applied"`
	FilterApplied bool     `json:"filter_applied,omitempty"`
	Message       string   `json:"message,omitempty"`
	Restrictions  []string `json:"restrictions,omitempty"`
}

func NewPermissionInfo() PermissionInfo {
	return PermissionInfo{Applied: false}
}

func NewPermissionInfoDenied(message string) PermissionInfo {
	return PermissionInfo{
		Applied: true,
		Message: message,
	}
}

func NewPermissionInfoFiltered(message string) PermissionInfo {
	return PermissionInfo{
		Applied:       true,
		FilterApplied: true,
		Message:       message,
	}
}

func NewPermissionInfoRestricted(restrictions []string) PermissionInfo {
	return PermissionInfo{
		Applied:      true,
		Restrictions: restrictions,
		Message:      fmt.Sprintf("%d items restricted by permissions", len(restrictions)),
	}
}

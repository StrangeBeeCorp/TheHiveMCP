package permissions

import (
	"reflect"
	"testing"
)

func TestMergeFilters(t *testing.T) {
	tests := []struct {
		name             string
		userQuery        map[string]interface{}
		permFilters      map[string]interface{}
		wantApplied      bool
		checkMergedField string
	}{
		{
			name:        "no permission filters",
			userQuery:   map[string]interface{}{"_field": "status", "_operator": "_eq", "_value": "Open"},
			permFilters: nil,
			wantApplied: false,
		},
		{
			name:        "empty permission filters",
			userQuery:   map[string]interface{}{"_field": "status"},
			permFilters: map[string]interface{}{},
			wantApplied: false,
		},
		{
			name:        "no user query",
			userQuery:   nil,
			permFilters: map[string]interface{}{"_field": "severity", "_operator": "_gte", "_value": 2},
			wantApplied: true,
		},
		{
			name:             "both exist - should merge with AND",
			userQuery:        map[string]interface{}{"_field": "status", "_operator": "_eq", "_value": "Open"},
			permFilters:      map[string]interface{}{"_field": "severity", "_operator": "_gte", "_value": 2},
			wantApplied:      true,
			checkMergedField: "_and",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged, applied := MergeFilters(tt.userQuery, tt.permFilters)

			if applied != tt.wantApplied {
				t.Errorf("MergeFilters() applied = %v, want %v", applied, tt.wantApplied)
			}

			if tt.checkMergedField != "" {
				if _, ok := merged[tt.checkMergedField]; !ok {
					t.Errorf("Expected merged filter to have field %s", tt.checkMergedField)
				}
			}
		})
	}
}

func TestApplyFiltersToQuery(t *testing.T) {
	tests := []struct {
		name        string
		query       map[string]interface{}
		permFilters map[string]interface{}
		wantApplied bool
		wantErr     bool
	}{
		{
			name:        "no permission filters",
			query:       map[string]interface{}{"query": map[string]interface{}{"_field": "status"}},
			permFilters: nil,
			wantApplied: false,
			wantErr:     false,
		},
		{
			name:        "with permission filters",
			query:       map[string]interface{}{"query": map[string]interface{}{"_field": "status"}},
			permFilters: map[string]interface{}{"_field": "severity", "_operator": "_gte", "_value": 2},
			wantApplied: true,
			wantErr:     false,
		},
		{
			name:        "nil query with filters",
			query:       nil,
			permFilters: map[string]interface{}{"_field": "severity"},
			wantApplied: true,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultQuery, applied, err := ApplyFiltersToQuery(tt.query, tt.permFilters)

			if (err != nil) != tt.wantErr {
				t.Errorf("ApplyFiltersToQuery() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if applied != tt.wantApplied {
				t.Errorf("ApplyFiltersToQuery() applied = %v, want %v", applied, tt.wantApplied)
			}

			if applied && resultQuery != nil {
				if _, ok := resultQuery["query"]; !ok {
					t.Error("Expected result query to have 'query' field")
				}
			}
		})
	}
}

func TestValidateFilter(t *testing.T) {
	tests := []struct {
		name    string
		filter  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "nil filter",
			filter:  nil,
			wantErr: false,
		},
		{
			name:    "empty filter",
			filter:  map[string]interface{}{},
			wantErr: false,
		},
		{
			name: "valid filter",
			filter: map[string]interface{}{
				"_field":    "status",
				"_operator": "_eq",
				"_value":    "Open",
			},
			wantErr: false,
		},
		{
			name: "valid complex filter",
			filter: map[string]interface{}{
				"_and": []interface{}{
					map[string]interface{}{"_field": "status", "_operator": "_eq", "_value": "Open"},
					map[string]interface{}{"_field": "severity", "_operator": "_gte", "_value": 2},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFilter(tt.filter)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFilter() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewPermissionInfo(t *testing.T) {
	info := NewPermissionInfo()
	if info.Applied {
		t.Error("NewPermissionInfo() should have Applied=false")
	}
}

func TestNewPermissionInfoDenied(t *testing.T) {
	msg := "access denied"
	info := NewPermissionInfoDenied(msg)

	if !info.Applied {
		t.Error("NewPermissionInfoDenied() should have Applied=true")
	}
	if info.Message != msg {
		t.Errorf("NewPermissionInfoDenied() message = %q, want %q", info.Message, msg)
	}
}

func TestNewPermissionInfoFiltered(t *testing.T) {
	msg := "filter applied"
	info := NewPermissionInfoFiltered(msg)

	if !info.Applied {
		t.Error("NewPermissionInfoFiltered() should have Applied=true")
	}
	if !info.FilterApplied {
		t.Error("NewPermissionInfoFiltered() should have FilterApplied=true")
	}
	if info.Message != msg {
		t.Errorf("NewPermissionInfoFiltered() message = %q, want %q", info.Message, msg)
	}
}

func TestNewPermissionInfoRestricted(t *testing.T) {
	restrictions := []string{"item1", "item2", "item3"}
	info := NewPermissionInfoRestricted(restrictions)

	if !info.Applied {
		t.Error("NewPermissionInfoRestricted() should have Applied=true")
	}
	if !reflect.DeepEqual(info.Restrictions, restrictions) {
		t.Errorf("NewPermissionInfoRestricted() restrictions = %v, want %v", info.Restrictions, restrictions)
	}
	if info.Message != "3 items restricted by permissions" {
		t.Errorf("NewPermissionInfoRestricted() message = %q, want '3 items restricted by permissions'", info.Message)
	}
}

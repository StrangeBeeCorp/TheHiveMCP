package permissions

import (
	"reflect"
	"testing"
)

func TestMergeFilters(t *testing.T) {
	tests := []struct {
		name             string
		userQuery        map[string]any
		permFilters      map[string]any
		wantApplied      bool
		checkMergedField string
	}{
		{
			name:        "no permission filters",
			userQuery:   map[string]any{testFieldKey: testFieldStatus, testOperatorKey: "_eq", testValueKey: "Open"},
			permFilters: nil,
			wantApplied: false,
		},
		{
			name:        "empty permission filters",
			userQuery:   map[string]any{testFieldKey: testFieldStatus},
			permFilters: map[string]any{},
			wantApplied: false,
		},
		{
			name:        "no user query",
			userQuery:   nil,
			permFilters: map[string]any{testFieldKey: "severity", testOperatorKey: "_gte", testValueKey: 2},
			wantApplied: true,
		},
		{
			name:             "both exist - should merge with AND",
			userQuery:        map[string]any{testFieldKey: testFieldStatus, testOperatorKey: "_eq", testValueKey: "Open"},
			permFilters:      map[string]any{testFieldKey: "severity", testOperatorKey: "_gte", testValueKey: 2},
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

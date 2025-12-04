package permissions

import (
	"testing"
)

func TestIsToolAllowed(t *testing.T) {
	config := &Config{
		Version: "1.0",
		Permissions: PermissionsSection{
			Tools: map[string]ToolPermission{
				"search-entities": {Allowed: true},
				"manage-entities": {Allowed: false},
			},
		},
	}

	tests := []struct {
		name     string
		toolName string
		want     bool
	}{
		{"allowed tool", "search-entities", true},
		{"denied tool", "manage-entities", false},
		{"nonexistent tool", "nonexistent", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := config.IsToolAllowed(tt.toolName); got != tt.want {
				t.Errorf("IsToolAllowed(%q) = %v, want %v", tt.toolName, got, tt.want)
			}
		})
	}
}

func TestIsAnalyzerAllowed_AllowList(t *testing.T) {
	config := &Config{
		Permissions: PermissionsSection{
			Analyzers: AutomationPermissions{
				Mode:    "allow_list",
				Allowed: []string{"VirusTotal", "Shodan"},
			},
		},
	}

	tests := []struct {
		name         string
		analyzerName string
		want         bool
	}{
		{"allowed analyzer", "VirusTotal", true},
		{"allowed analyzer", "Shodan", true},
		{"denied analyzer", "MISP", false},
		{"empty analyzer", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := config.IsAnalyzerAllowed(tt.analyzerName, ""); got != tt.want {
				t.Errorf("IsAnalyzerAllowed(%q) = %v, want %v", tt.analyzerName, got, tt.want)
			}
		})
	}
}

func TestIsAnalyzerAllowed_BlockList(t *testing.T) {
	config := &Config{
		Permissions: PermissionsSection{
			Analyzers: AutomationPermissions{
				Mode:    "block_list",
				Blocked: []string{"BadAnalyzer"},
			},
		},
	}

	tests := []struct {
		name         string
		analyzerName string
		want         bool
	}{
		{"not blocked analyzer", "VirusTotal", true},
		{"blocked analyzer", "BadAnalyzer", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := config.IsAnalyzerAllowed(tt.analyzerName, ""); got != tt.want {
				t.Errorf("IsAnalyzerAllowed(%q) = %v, want %v", tt.analyzerName, got, tt.want)
			}
		})
	}
}

func TestIsAnalyzerAllowed_Wildcard(t *testing.T) {
	config := &Config{
		Permissions: PermissionsSection{
			Analyzers: AutomationPermissions{
				Mode:    "allow_list",
				Allowed: []string{"*"},
			},
		},
	}

	if !config.IsAnalyzerAllowed("AnyAnalyzer", "") {
		t.Error("Wildcard should allow any analyzer")
	}
}

func TestIsAnalyzerAllowed_ToolSpecific(t *testing.T) {
	config := &Config{
		Permissions: PermissionsSection{
			Tools: map[string]ToolPermission{
				"execute-automation": {
					Allowed: true,
					AnalyzerRestrictions: &AutomationRestrictions{
						Mode:    "allow_list",
						Allowed: []string{"ToolSpecificAnalyzer"},
					},
				},
			},
			Analyzers: AutomationPermissions{
				Mode:    "allow_list",
				Allowed: []string{"GlobalAnalyzer"},
			},
		},
	}

	// Tool-specific should override global
	if !config.IsAnalyzerAllowed("ToolSpecificAnalyzer", "execute-automation") {
		t.Error("Tool-specific analyzer should be allowed")
	}

	if config.IsAnalyzerAllowed("GlobalAnalyzer", "execute-automation") {
		t.Error("Global analyzer should not be allowed when tool-specific is defined")
	}

	// Without tool name, use global
	if !config.IsAnalyzerAllowed("GlobalAnalyzer", "") {
		t.Error("Global analyzer should be allowed without tool name")
	}
}

func TestIsResponderAllowed_AllowList(t *testing.T) {
	config := &Config{
		Permissions: PermissionsSection{
			Responders: AutomationPermissions{
				Mode:    "allow_list",
				Allowed: []string{"Responder1", "Responder2"},
			},
		},
	}

	tests := []struct {
		name          string
		responderName string
		want          bool
	}{
		{"allowed responder", "Responder1", true},
		{"denied responder", "Responder3", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := config.IsResponderAllowed(tt.responderName, ""); got != tt.want {
				t.Errorf("IsResponderAllowed(%q) = %v, want %v", tt.responderName, got, tt.want)
			}
		})
	}
}

func TestGetAllowedAnalyzers(t *testing.T) {
	config := &Config{
		Permissions: PermissionsSection{
			Analyzers: AutomationPermissions{
				Mode:    "allow_list",
				Allowed: []string{"Analyzer1", "Analyzer2"},
			},
		},
	}

	allAnalyzers := []string{"Analyzer1", "Analyzer2", "Analyzer3", "Analyzer4"}
	allowed := config.GetAllowedAnalyzers(allAnalyzers, "")

	if len(allowed) != 2 {
		t.Errorf("Expected 2 allowed analyzers, got %d", len(allowed))
	}

	expected := map[string]bool{"Analyzer1": true, "Analyzer2": true}
	for _, name := range allowed {
		if !expected[name] {
			t.Errorf("Unexpected analyzer in allowed list: %s", name)
		}
	}
}

func TestGetAllowedResponders(t *testing.T) {
	config := &Config{
		Permissions: PermissionsSection{
			Responders: AutomationPermissions{
				Mode:    "block_list",
				Blocked: []string{"BadResponder"},
			},
		},
	}

	allResponders := []string{"Responder1", "BadResponder", "Responder2"}
	allowed := config.GetAllowedResponders(allResponders, "")

	if len(allowed) != 2 {
		t.Errorf("Expected 2 allowed responders, got %d", len(allowed))
	}

	for _, name := range allowed {
		if name == "BadResponder" {
			t.Error("BadResponder should not be in allowed list")
		}
	}
}

func TestIsEntityOperationAllowed(t *testing.T) {
	tests := []struct {
		name       string
		config     *Config
		entityType string
		operation  string
		want       bool
	}{
		{
			name: "entity operation allowed",
			config: &Config{
				Version: "1.0",
				Permissions: PermissionsSection{
					Tools: map[string]ToolPermission{
						"manage-entities": {
							Allowed: true,
							EntityPermissions: map[string]EntityOperation{
								"alert": {
									Create:  true,
									Update:  true,
									Delete:  false,
									Comment: true,
								},
							},
						},
					},
				},
			},
			entityType: "alert",
			operation:  "create",
			want:       true,
		},
		{
			name: "entity operation denied",
			config: &Config{
				Version: "1.0",
				Permissions: PermissionsSection{
					Tools: map[string]ToolPermission{
						"manage-entities": {
							Allowed: true,
							EntityPermissions: map[string]EntityOperation{
								"alert": {
									Create:  true,
									Update:  true,
									Delete:  false,
									Comment: true,
								},
							},
						},
					},
				},
			},
			entityType: "alert",
			operation:  "delete",
			want:       false,
		},
		{
			name: "entity type not configured - should deny",
			config: &Config{
				Version: "1.0",
				Permissions: PermissionsSection{
					Tools: map[string]ToolPermission{
						"manage-entities": {
							Allowed: true,
							EntityPermissions: map[string]EntityOperation{
								"alert": {
									Create: true,
								},
							},
						},
					},
				},
			},
			entityType: "case",
			operation:  "create",
			want:       false,
		},
		{
			name: "no entity permissions configured - allow all (backward compatibility)",
			config: &Config{
				Version: "1.0",
				Permissions: PermissionsSection{
					Tools: map[string]ToolPermission{
						"manage-entities": {
							Allowed: true,
						},
					},
				},
			},
			entityType: "alert",
			operation:  "create",
			want:       true,
		},
		{
			name: "tool not allowed",
			config: &Config{
				Version: "1.0",
				Permissions: PermissionsSection{
					Tools: map[string]ToolPermission{
						"manage-entities": {
							Allowed: false,
						},
					},
				},
			},
			entityType: "alert",
			operation:  "create",
			want:       false,
		},
		{
			name: "comment operation allowed",
			config: &Config{
				Version: "1.0",
				Permissions: PermissionsSection{
					Tools: map[string]ToolPermission{
						"manage-entities": {
							Allowed: true,
							EntityPermissions: map[string]EntityOperation{
								"case": {
									Create:  false,
									Update:  false,
									Delete:  false,
									Comment: true,
								},
							},
						},
					},
				},
			},
			entityType: "case",
			operation:  "comment",
			want:       true,
		},
		{
			name: "invalid operation",
			config: &Config{
				Version: "1.0",
				Permissions: PermissionsSection{
					Tools: map[string]ToolPermission{
						"manage-entities": {
							Allowed: true,
							EntityPermissions: map[string]EntityOperation{
								"alert": {
									Create:  true,
									Update:  true,
									Delete:  true,
									Comment: true,
								},
							},
						},
					},
				},
			},
			entityType: "alert",
			operation:  "invalid",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.IsEntityOperationAllowed(tt.entityType, tt.operation)
			if got != tt.want {
				t.Errorf("IsEntityOperationAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}

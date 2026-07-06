package permissions

import (
	"testing"
)

func TestIsToolAllowed(t *testing.T) {
	config := &Config{
		Version: versionV1,
		Permissions: Section{
			Tools: map[string]ToolPermission{
				toolSearchEntities: {Allowed: true},
				toolManageEntities: {Allowed: false},
			},
		},
	}

	tests := []struct {
		name     string
		toolName string
		want     bool
	}{
		{"allowed tool", toolSearchEntities, true},
		{"denied tool", toolManageEntities, false},
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
		Permissions: Section{
			Analyzers: AutomationPermissions{
				Mode:    modeAllowList,
				Allowed: []string{testAnalyzerVirusTotal, "Shodan"},
			},
		},
	}

	tests := []struct {
		name         string
		analyzerName string
		want         bool
	}{
		{"allowed analyzer", testAnalyzerVirusTotal, true},
		{"allowed analyzer", "Shodan", true},
		{"denied analyzer", "MISP", false},
		{"empty analyzer", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := config.IsAnalyzerAllowed(tt.analyzerName); got != tt.want {
				t.Errorf("IsAnalyzerAllowed(%q) = %v, want %v", tt.analyzerName, got, tt.want)
			}
		})
	}
}

func TestIsAnalyzerAllowed_BlockList(t *testing.T) {
	config := &Config{
		Permissions: Section{
			Analyzers: AutomationPermissions{
				Mode:    modeBlockList,
				Blocked: []string{"BadAnalyzer"},
			},
		},
	}

	tests := []struct {
		name         string
		analyzerName string
		want         bool
	}{
		{"not blocked analyzer", testAnalyzerVirusTotal, true},
		{"blocked analyzer", "BadAnalyzer", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := config.IsAnalyzerAllowed(tt.analyzerName); got != tt.want {
				t.Errorf("IsAnalyzerAllowed(%q) = %v, want %v", tt.analyzerName, got, tt.want)
			}
		})
	}
}

func TestIsAnalyzerAllowed_Wildcard(t *testing.T) {
	config := &Config{
		Permissions: Section{
			Analyzers: AutomationPermissions{
				Mode:    modeAllowList,
				Allowed: []string{"*"},
			},
		},
	}

	if !config.IsAnalyzerAllowed("AnyAnalyzer") {
		t.Error("Wildcard should allow any analyzer")
	}
}

func TestIsResponderAllowed_AllowList(t *testing.T) {
	config := &Config{
		Permissions: Section{
			Responders: AutomationPermissions{
				Mode:    modeAllowList,
				Allowed: []string{testResponder1, "Responder2"},
			},
		},
	}

	tests := []struct {
		name          string
		responderName string
		want          bool
	}{
		{"allowed responder", testResponder1, true},
		{"denied responder", "Responder3", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := config.IsResponderAllowed(tt.responderName); got != tt.want {
				t.Errorf("IsResponderAllowed(%q) = %v, want %v", tt.responderName, got, tt.want)
			}
		})
	}
}

func TestGetAllowedAnalyzers(t *testing.T) {
	config := &Config{
		Permissions: Section{
			Analyzers: AutomationPermissions{
				Mode:    modeAllowList,
				Allowed: []string{testAnalyzer1, testAnalyzer2},
			},
		},
	}

	allAnalyzers := []string{testAnalyzer1, testAnalyzer2, "Analyzer3", "Analyzer4"}
	allowed := config.GetAllowedAnalyzers(allAnalyzers)

	if len(allowed) != 2 {
		t.Errorf("Expected 2 allowed analyzers, got %d", len(allowed))
	}

	expected := map[string]bool{testAnalyzer1: true, testAnalyzer2: true}
	for _, name := range allowed {
		if !expected[name] {
			t.Errorf("Unexpected analyzer in allowed list: %s", name)
		}
	}
}

func TestGetAllowedResponders(t *testing.T) {
	config := &Config{
		Permissions: Section{
			Responders: AutomationPermissions{
				Mode:    modeBlockList,
				Blocked: []string{testBadResponder},
			},
		},
	}

	allResponders := []string{testResponder1, testBadResponder, "Responder2"}
	allowed := config.GetAllowedResponders(allResponders)

	if len(allowed) != 2 {
		t.Errorf("Expected 2 allowed responders, got %d", len(allowed))
	}

	for _, name := range allowed {
		if name == testBadResponder {
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
				Version: versionV1,
				Permissions: Section{
					Tools: map[string]ToolPermission{
						toolManageEntities: {
							Allowed: true,
							EntityPermissions: map[string]EntityOperation{
								testEntityAlert: {
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
			entityType: testEntityAlert,
			operation:  operationCreate,
			want:       true,
		},
		{
			name: "entity operation denied",
			config: &Config{
				Version: versionV1,
				Permissions: Section{
					Tools: map[string]ToolPermission{
						toolManageEntities: {
							Allowed: true,
							EntityPermissions: map[string]EntityOperation{
								testEntityAlert: {
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
			entityType: testEntityAlert,
			operation:  "delete",
			want:       false,
		},
		{
			name: "entity type not configured - should deny",
			config: &Config{
				Version: versionV1,
				Permissions: Section{
					Tools: map[string]ToolPermission{
						toolManageEntities: {
							Allowed: true,
							EntityPermissions: map[string]EntityOperation{
								testEntityAlert: {
									Create: true,
								},
							},
						},
					},
				},
			},
			entityType: testEntityCase,
			operation:  operationCreate,
			want:       false,
		},
		{
			name: "no entity permissions configured - allow all (backward compatibility)",
			config: &Config{
				Version: versionV1,
				Permissions: Section{
					Tools: map[string]ToolPermission{
						toolManageEntities: {
							Allowed: true,
						},
					},
				},
			},
			entityType: testEntityAlert,
			operation:  operationCreate,
			want:       true,
		},
		{
			name: "tool not allowed",
			config: &Config{
				Version: versionV1,
				Permissions: Section{
					Tools: map[string]ToolPermission{
						toolManageEntities: {
							Allowed: false,
						},
					},
				},
			},
			entityType: testEntityAlert,
			operation:  operationCreate,
			want:       false,
		},
		{
			name: "comment operation allowed",
			config: &Config{
				Version: versionV1,
				Permissions: Section{
					Tools: map[string]ToolPermission{
						toolManageEntities: {
							Allowed: true,
							EntityPermissions: map[string]EntityOperation{
								testEntityCase: {
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
			entityType: testEntityCase,
			operation:  "comment",
			want:       true,
		},
		{
			name: "invalid operation",
			config: &Config{
				Version: versionV1,
				Permissions: Section{
					Tools: map[string]ToolPermission{
						toolManageEntities: {
							Allowed: true,
							EntityPermissions: map[string]EntityOperation{
								testEntityAlert: {
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
			entityType: testEntityAlert,
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

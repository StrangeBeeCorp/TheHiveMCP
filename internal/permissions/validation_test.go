package permissions

import (
	"testing"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				Version: versionV1,
				Permissions: Section{
					Tools: map[string]ToolPermission{
						toolSearchEntities: {Allowed: true},
					},
					Analyzers: AutomationPermissions{
						Mode:    modeAllowList,
						Allowed: []string{testAutomationTest},
					},
					Responders: AutomationPermissions{
						Mode:    modeBlockList,
						Blocked: []string{testAutomationBad},
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "missing version",
			config: &Config{
				Permissions: Section{},
			},
			wantErr: true,
		},
		{
			name: "invalid version",
			config: &Config{
				Version:     "2.0",
				Permissions: Section{},
			},
			wantErr: true,
		},
		{
			name: "unknown tool",
			config: &Config{
				Version: versionV1,
				Permissions: Section{
					Tools: map[string]ToolPermission{
						"unknown-tool": {Allowed: true},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid analyzer mode",
			config: &Config{
				Version: versionV1,
				Permissions: Section{
					Analyzers: AutomationPermissions{
						Mode: "invalid_mode",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "allow_list with blocked items",
			config: &Config{
				Version: versionV1,
				Permissions: Section{
					Analyzers: AutomationPermissions{
						Mode:    modeAllowList,
						Allowed: []string{testAutomationTest},
						Blocked: []string{testAutomationBad},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "block_list with allowed items",
			config: &Config{
				Version: versionV1,
				Permissions: Section{
					Responders: AutomationPermissions{
						Mode:    modeBlockList,
						Allowed: []string{testAutomationTest},
						Blocked: []string{testAutomationBad},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := Validate(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

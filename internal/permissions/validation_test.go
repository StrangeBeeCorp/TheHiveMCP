package permissions

import (
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				Version: "1.0",
				Permissions: PermissionsSection{
					Tools: map[string]ToolPermission{
						"search-entities": {Allowed: true},
					},
					Analyzers: AutomationPermissions{
						Mode:    "allow_list",
						Allowed: []string{"Test"},
					},
					Responders: AutomationPermissions{
						Mode:    "block_list",
						Blocked: []string{"Bad"},
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
				Permissions: PermissionsSection{},
			},
			wantErr: true,
		},
		{
			name: "invalid version",
			config: &Config{
				Version:     "2.0",
				Permissions: PermissionsSection{},
			},
			wantErr: true,
		},
		{
			name: "unknown tool",
			config: &Config{
				Version: "1.0",
				Permissions: PermissionsSection{
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
				Version: "1.0",
				Permissions: PermissionsSection{
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
				Version: "1.0",
				Permissions: PermissionsSection{
					Analyzers: AutomationPermissions{
						Mode:    "allow_list",
						Allowed: []string{"Test"},
						Blocked: []string{"Bad"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "block_list with allowed items",
			config: &Config{
				Version: "1.0",
				Permissions: PermissionsSection{
					Responders: AutomationPermissions{
						Mode:    "block_list",
						Allowed: []string{"Test"},
						Blocked: []string{"Bad"},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

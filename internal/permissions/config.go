package permissions

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadFromFile loads and parses a permissions configuration from a file
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path comes from trusted CLI argument
	if err != nil {
		return nil, fmt.Errorf("failed to read permissions file: %w", err)
	}

	config, err := ParseYAML(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse permissions file: %w", err)
	}

	err = Validate(config)
	if err != nil {
		return nil, fmt.Errorf("invalid permissions configuration: %w", err)
	}

	return config, nil
}

// LoadDefault loads the embedded default read-only permissions
func LoadDefault() (*Config, error) {
	data, err := GetDefaultPermissions()
	if err != nil {
		return nil, fmt.Errorf("failed to load default permissions: %w", err)
	}

	config, err := ParseYAML(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse default permissions: %w", err)
	}

	return config, nil
}

// LoadAdminForTesting returns an admin permissions configuration for testing purposes
func LoadAdminForTesting() *Config {
	return &Config{
		Version: versionV1,
		Permissions: Section{
			Tools: map[string]ToolPermission{
				toolSearchEntities:   {Allowed: true},
				toolManageEntities:   {Allowed: true},
				"execute-automation": {Allowed: true},
				"get-resource":       {Allowed: true},
			},
			Analyzers: AutomationPermissions{
				Mode:    modeAllowList,
				Allowed: []string{"*"},
			},
			Responders: AutomationPermissions{
				Mode:    modeAllowList,
				Allowed: []string{"*"},
			},
		},
	}
}

// ParseYAML parses YAML data into a Config struct
func ParseYAML(data []byte) (*Config, error) {
	var config Config

	err := yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	return &config, nil
}

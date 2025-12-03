package permissions

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadFromFile loads and parses a permissions configuration from a file
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read permissions file: %w", err)
	}

	config, err := ParseYAML(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse permissions file: %w", err)
	}

	if err := Validate(config); err != nil {
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

// ParseYAML parses YAML data into a Config struct
func ParseYAML(data []byte) (*Config, error) {
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}
	return &config, nil
}

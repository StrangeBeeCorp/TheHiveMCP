package permissions

import (
	"fmt"
)

func Validate(config *Config) error {
	if config == nil {
		return fmt.Errorf("config is nil")
	}

	if config.Version == "" {
		return fmt.Errorf("version is required")
	}
	if config.Version != "1.0" {
		return fmt.Errorf("unsupported version: %s (supported: 1.0)", config.Version)
	}

	if err := validateTools(config.Permissions.Tools); err != nil {
		return fmt.Errorf("invalid tools configuration: %w", err)
	}

	if err := validateAutomationPermissions("analyzers", config.Permissions.Analyzers); err != nil {
		return err
	}

	if err := validateAutomationPermissions("responders", config.Permissions.Responders); err != nil {
		return err
	}

	return nil
}

func validateTools(tools map[string]ToolPermission) error {
	validTools := map[string]bool{
		"search-entities":    true,
		"manage-entities":    true,
		"execute-automation": true,
		"get-resource":       true,
	}

	for toolName := range tools {
		if !validTools[toolName] {
			return fmt.Errorf("unknown tool: %s", toolName)
		}
	}

	return nil
}

func validateAutomationPermissions(name string, perms AutomationPermissions) error {
	if perms.Mode != "" && perms.Mode != "allow_list" && perms.Mode != "block_list" {
		return fmt.Errorf("invalid %s mode: %s (must be 'allow_list' or 'block_list')", name, perms.Mode)
	}

	if perms.Mode == "allow_list" && len(perms.Blocked) > 0 {
		return fmt.Errorf("%s: cannot specify 'blocked' list when mode is 'allow_list'", name)
	}

	if perms.Mode == "block_list" && len(perms.Allowed) > 0 {
		return fmt.Errorf("%s: cannot specify 'allowed' list when mode is 'block_list'", name)
	}

	return nil
}

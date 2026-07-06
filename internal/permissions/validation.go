package permissions

import (
	"errors"
	"fmt"
)

// Validate validates a permissions configuration
func Validate(config *Config) error {
	if config == nil {
		return errors.New("config is nil")
	}

	// Validate version
	if config.Version == "" {
		return errors.New("version is required")
	}

	if config.Version != versionV1 {
		return fmt.Errorf("unsupported version: %s (supported: %s)", config.Version, versionV1)
	}

	// Validate tools
	err := validateTools(config.Permissions.Tools)
	if err != nil {
		return fmt.Errorf("invalid tools configuration: %w", err)
	}

	// Validate analyzers
	err = validateAutomationPermissions("analyzers", config.Permissions.Analyzers)
	if err != nil {
		return err
	}

	// Validate responders
	err = validateAutomationPermissions("responders", config.Permissions.Responders)
	if err != nil {
		return err
	}

	return nil
}

// validateTools validates tool permissions
func validateTools(tools map[string]ToolPermission) error {
	validTools := map[string]bool{
		toolSearchEntities:   true,
		toolManageEntities:   true,
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

// validateAutomationPermissions validates analyzer or responder permissions
func validateAutomationPermissions(name string, perms AutomationPermissions) error {
	if perms.Mode != "" && perms.Mode != modeAllowList && perms.Mode != modeBlockList {
		return fmt.Errorf("invalid %s mode: %s (must be 'allow_list' or 'block_list')", name, perms.Mode)
	}

	if perms.Mode == modeAllowList && len(perms.Blocked) > 0 {
		return fmt.Errorf("%s: cannot specify 'blocked' list when mode is 'allow_list'", name)
	}

	if perms.Mode == modeBlockList && len(perms.Allowed) > 0 {
		return fmt.Errorf("%s: cannot specify 'allowed' list when mode is 'block_list'", name)
	}

	return nil
}

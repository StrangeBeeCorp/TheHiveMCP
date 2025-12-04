package permissions

import (
	"embed"
	"fmt"
)

//go:embed embedded/*.yaml
var embeddedFS embed.FS

// GetDefaultPermissions returns the embedded default read-only permissions
func GetDefaultPermissions() ([]byte, error) {
	data, err := embeddedFS.ReadFile("embedded/default_permissions.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to read default permissions: %w", err)
	}
	return data, nil
}

// Config represents the complete permissions configuration
type Config struct {
	Version     string             `yaml:"version"`
	Permissions PermissionsSection `yaml:"permissions"`
}

// PermissionsSection contains all permission categories
type PermissionsSection struct {
	Tools      map[string]ToolPermission `yaml:"tools"`
	Analyzers  AutomationPermissions     `yaml:"analyzers"`
	Responders AutomationPermissions     `yaml:"responders"`
}

// ToolPermission defines access and filtering for a specific tool
type ToolPermission struct {
	Allowed               bool                       `yaml:"allowed"`
	Filters               map[string]interface{}     `yaml:"filters,omitempty"`
	AnalyzerRestrictions  *AutomationRestrictions    `yaml:"analyzer_restrictions,omitempty"`
	ResponderRestrictions *AutomationRestrictions    `yaml:"responder_restrictions,omitempty"`
	EntityPermissions     map[string]EntityOperation `yaml:"entity_permissions,omitempty"` // For manage-entities tool
}

// EntityOperation defines which operations are allowed for an entity type
type EntityOperation struct {
	Create  bool `yaml:"create"`
	Update  bool `yaml:"update"`
	Delete  bool `yaml:"delete"`
	Comment bool `yaml:"comment"`
}

// AutomationPermissions defines analyzer or responder access
type AutomationPermissions struct {
	Mode    string   `yaml:"mode"` // "allow_list" or "block_list"
	Allowed []string `yaml:"allowed"`
	Blocked []string `yaml:"blocked"`
}

// AutomationRestrictions defines per-tool automation restrictions
type AutomationRestrictions struct {
	Mode    string   `yaml:"mode"` // "allow_list" or "block_list"
	Allowed []string `yaml:"allowed"`
	Blocked []string `yaml:"blocked"`
}

// IsToolAllowed checks if a tool is permitted
func (c *Config) IsToolAllowed(toolName string) bool {
	if c == nil || c.Permissions.Tools == nil {
		return false
	}
	perm, exists := c.Permissions.Tools[toolName]
	if !exists {
		return false
	}
	return perm.Allowed
}

// IsEntityOperationAllowed checks if a specific operation on an entity type is permitted
// If no entity-specific permissions are configured, defaults to the tool's general allowed setting
func (c *Config) IsEntityOperationAllowed(entityType, operation string) bool {
	if c == nil || c.Permissions.Tools == nil {
		return false
	}

	toolPerm, exists := c.Permissions.Tools["manage-entities"]
	if !exists || !toolPerm.Allowed {
		return false
	}

	// If no entity permissions configured, allow all operations (backward compatibility)
	if len(toolPerm.EntityPermissions) == 0 {
		return true
	}

	// Check entity-specific permissions
	entityPerm, exists := toolPerm.EntityPermissions[entityType]
	if !exists {
		// If entity type not specified, deny by default
		return false
	}

	// Check operation permission
	switch operation {
	case "create":
		return entityPerm.Create
	case "update":
		return entityPerm.Update
	case "delete":
		return entityPerm.Delete
	case "comment":
		return entityPerm.Comment
	default:
		return false
	}
}

// GetToolFilters returns the filters for a specific tool
func (c *Config) GetToolFilters(toolName string) map[string]interface{} {
	if c == nil || c.Permissions.Tools == nil {
		return nil
	}
	perm, exists := c.Permissions.Tools[toolName]
	if !exists {
		return nil
	}
	return perm.Filters
}

// IsAnalyzerAllowed checks if an analyzer is permitted based on global or tool-specific rules
func (c *Config) IsAnalyzerAllowed(analyzerName string, toolName string) bool {
	if c == nil {
		return false
	}

	// Check tool-specific restrictions first
	if toolName != "" && c.Permissions.Tools != nil {
		if perm, exists := c.Permissions.Tools[toolName]; exists && perm.AnalyzerRestrictions != nil {
			return isAutomationAllowed(analyzerName, perm.AnalyzerRestrictions.Mode, perm.AnalyzerRestrictions.Allowed, perm.AnalyzerRestrictions.Blocked)
		}
	}

	// Fall back to global analyzer permissions
	return isAutomationAllowed(analyzerName, c.Permissions.Analyzers.Mode, c.Permissions.Analyzers.Allowed, c.Permissions.Analyzers.Blocked)
}

// IsResponderAllowed checks if a responder is permitted based on global or tool-specific rules
func (c *Config) IsResponderAllowed(responderName string, toolName string) bool {
	if c == nil {
		return false
	}

	// Check tool-specific restrictions first
	if toolName != "" && c.Permissions.Tools != nil {
		if perm, exists := c.Permissions.Tools[toolName]; exists && perm.ResponderRestrictions != nil {
			return isAutomationAllowed(responderName, perm.ResponderRestrictions.Mode, perm.ResponderRestrictions.Allowed, perm.ResponderRestrictions.Blocked)
		}
	}

	// Fall back to global responder permissions
	return isAutomationAllowed(responderName, c.Permissions.Responders.Mode, c.Permissions.Responders.Allowed, c.Permissions.Responders.Blocked)
}

// GetAllowedAnalyzers returns list of allowed analyzer names
func (c *Config) GetAllowedAnalyzers(allAnalyzers []string, toolName string) []string {
	if c == nil {
		return []string{}
	}

	var allowed []string
	for _, analyzer := range allAnalyzers {
		if c.IsAnalyzerAllowed(analyzer, toolName) {
			allowed = append(allowed, analyzer)
		}
	}
	return allowed
}

// GetAllowedResponders returns list of allowed responder names
func (c *Config) GetAllowedResponders(allResponders []string, toolName string) []string {
	if c == nil {
		return []string{}
	}

	var allowed []string
	for _, responder := range allResponders {
		if c.IsResponderAllowed(responder, toolName) {
			allowed = append(allowed, responder)
		}
	}
	return allowed
}

// isAutomationAllowed checks if an automation item is allowed based on mode and lists
func isAutomationAllowed(name, mode string, allowed, blocked []string) bool {
	switch mode {
	case "allow_list":
		if len(allowed) == 0 {
			return false
		}
		// Check for wildcard
		for _, a := range allowed {
			if a == "*" {
				return true
			}
		}
		// Check if explicitly allowed
		for _, a := range allowed {
			if a == name {
				return true
			}
		}
		return false

	case "block_list":
		// Check if explicitly blocked
		for _, b := range blocked {
			if b == name {
				return false
			}
		}
		return true

	default:
		return false
	}
}

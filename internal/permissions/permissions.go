// Package permissions defines the permissions configuration model and the
// access-control checks (tools, entity operations, analyzers, responders and
// filters) applied to TheHive MCP requests.
package permissions

import (
	"embed"
	"fmt"
	"slices"
)

// Configuration schema version and permission modes.
const (
	// versionV1 is the only supported permissions schema version.
	versionV1 = "1.0"
	// modeAllowList only permits items listed in the allowed list.
	modeAllowList = "allow_list"
	// modeBlockList permits every item except those in the blocked list.
	modeBlockList = "block_list"
)

// Tool names recognized in the permissions configuration.
const (
	toolSearchEntities = "search-entities"
	toolManageEntities = "manage-entities"
)

// operationCreate is the entity "create" operation name.
const operationCreate = "create"

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
	Version     string  `yaml:"version"`
	Permissions Section `yaml:"permissions"`
}

// Section contains all permission categories
type Section struct {
	Tools      map[string]ToolPermission `yaml:"tools"`
	Analyzers  AutomationPermissions     `yaml:"analyzers"`
	Responders AutomationPermissions     `yaml:"responders"`
}

// ToolPermission defines access and filtering for a specific tool
type ToolPermission struct {
	Allowed           bool                       `yaml:"allowed"`
	Filters           map[string]any             `yaml:"filters,omitempty"`
	EntityPermissions map[string]EntityOperation `yaml:"entity_permissions,omitempty"` // For manage-entities tool
}

// EntityOperation defines which operations are allowed for an entity type
type EntityOperation struct {
	Create        bool `yaml:"create"`
	Update        bool `yaml:"update"`
	Delete        bool `yaml:"delete"`
	Comment       bool `yaml:"comment"`
	Promote       bool `yaml:"promote"`
	Merge         bool `yaml:"merge"`
	ApplyTemplate bool `yaml:"apply-template"`
}

// AutomationPermissions defines analyzer or responder access
type AutomationPermissions struct {
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

	toolPerm, exists := c.Permissions.Tools[toolManageEntities]
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
	case operationCreate:
		return entityPerm.Create
	case "update":
		return entityPerm.Update
	case "delete":
		return entityPerm.Delete
	case "comment":
		return entityPerm.Comment
	case "promote":
		return entityPerm.Promote
	case "merge":
		return entityPerm.Merge
	case "apply-template":
		return entityPerm.ApplyTemplate
	default:
		return false
	}
}

// GetToolFilters returns the filters for a specific tool
func (c *Config) GetToolFilters(toolName string) map[string]any {
	if c == nil || c.Permissions.Tools == nil {
		return nil
	}

	perm, exists := c.Permissions.Tools[toolName]
	if !exists {
		return nil
	}

	return perm.Filters
}

// IsAnalyzerAllowed checks if an analyzer is permitted based on global rules
func (c *Config) IsAnalyzerAllowed(analyzerName string) bool {
	if c == nil {
		return false
	}

	return isAutomationAllowed(analyzerName, c.Permissions.Analyzers.Mode, c.Permissions.Analyzers.Allowed, c.Permissions.Analyzers.Blocked)
}

// IsResponderAllowed checks if a responder is permitted based on global rules
func (c *Config) IsResponderAllowed(responderName string) bool {
	if c == nil {
		return false
	}

	return isAutomationAllowed(responderName, c.Permissions.Responders.Mode, c.Permissions.Responders.Allowed, c.Permissions.Responders.Blocked)
}

// GetAllowedAnalyzers returns list of allowed analyzer names
func (c *Config) GetAllowedAnalyzers(allAnalyzers []string) []string {
	if c == nil {
		return []string{}
	}

	var allowed []string

	for _, analyzer := range allAnalyzers {
		if c.IsAnalyzerAllowed(analyzer) {
			allowed = append(allowed, analyzer)
		}
	}

	return allowed
}

// GetAllowedResponders returns list of allowed responder names
func (c *Config) GetAllowedResponders(allResponders []string) []string {
	if c == nil {
		return []string{}
	}

	var allowed []string

	for _, responder := range allResponders {
		if c.IsResponderAllowed(responder) {
			allowed = append(allowed, responder)
		}
	}

	return allowed
}

// isAutomationAllowed checks if an automation item is allowed based on mode and lists
func isAutomationAllowed(name, mode string, allowed, blocked []string) bool {
	switch mode {
	case modeAllowList:
		if len(allowed) == 0 {
			return false
		}
		// Check for wildcard
		if slices.Contains(allowed, "*") {
			return true
		}
		// Check if explicitly allowed
		return slices.Contains(allowed, name)

	case modeBlockList:
		// Check if explicitly blocked
		return !slices.Contains(blocked, name)

	default:
		return false
	}
}

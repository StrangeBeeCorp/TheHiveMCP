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

type Config struct {
	Version     string             `yaml:"version"`
	Permissions PermissionsSection `yaml:"permissions"`
}

type PermissionsSection struct {
	Tools      map[string]ToolPermission `yaml:"tools"`
	Analyzers  AutomationPermissions     `yaml:"analyzers"`
	Responders AutomationPermissions     `yaml:"responders"`
}

type ToolPermission struct {
	Allowed           bool                       `yaml:"allowed"`
	Filters           map[string]interface{}     `yaml:"filters,omitempty"`
	EntityPermissions map[string]EntityOperation `yaml:"entity_permissions,omitempty"` // For manage-entities tool
}

type EntityOperation struct {
	Create        bool `yaml:"create"`
	Update        bool `yaml:"update"`
	Delete        bool `yaml:"delete"`
	Comment       bool `yaml:"comment"`
	Promote       bool `yaml:"promote"`
	Merge         bool `yaml:"merge"`
	ApplyTemplate bool `yaml:"apply-template"`
}

type AutomationPermissions struct {
	Mode    string   `yaml:"mode"` // "allow_list" or "block_list"
	Allowed []string `yaml:"allowed"`
	Blocked []string `yaml:"blocked"`
}

// IsToolAllowed reports whether toolName is permitted; unknown tools and a nil config deny.
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

// IsEntityOperationAllowed reports whether operation is permitted on entityType.
// With no entity-specific permissions configured, defaults to the tool's general allowed setting.
func (c *Config) IsEntityOperationAllowed(entityType, operation string) bool {
	if c == nil || c.Permissions.Tools == nil {
		return false
	}

	toolPerm, exists := c.Permissions.Tools["manage-entities"]
	if !exists || !toolPerm.Allowed {
		return false
	}

	// No entity permissions configured: allow all (backward compatibility).
	if len(toolPerm.EntityPermissions) == 0 {
		return true
	}

	entityPerm, exists := toolPerm.EntityPermissions[entityType]
	if !exists {
		return false // unlisted entity type: deny by default
	}

	switch operation {
	case "create":
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

func (c *Config) IsAnalyzerAllowed(analyzerName string) bool {
	if c == nil {
		return false
	}
	return isAutomationAllowed(analyzerName, c.Permissions.Analyzers.Mode, c.Permissions.Analyzers.Allowed, c.Permissions.Analyzers.Blocked)
}

func (c *Config) IsResponderAllowed(responderName string) bool {
	if c == nil {
		return false
	}
	return isAutomationAllowed(responderName, c.Permissions.Responders.Mode, c.Permissions.Responders.Allowed, c.Permissions.Responders.Blocked)
}

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

func isAutomationAllowed(name, mode string, allowed, blocked []string) bool {
	switch mode {
	case "allow_list":
		if len(allowed) == 0 {
			return false
		}
		for _, a := range allowed {
			if a == "*" {
				return true
			}
		}
		for _, a := range allowed {
			if a == name {
				return true
			}
		}
		return false

	case "block_list":
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

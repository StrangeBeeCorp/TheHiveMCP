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

	toolPerm, exists := c.Permissions.Tools[toolManageEntities]
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

// IsAnalyzerAllowed reports whether an analyzer may run. Pass every identifier
// the analyzer is known by, its ID and its name, since the rules may list either.
func (c *Config) IsAnalyzerAllowed(identifiers ...string) bool {
	if c == nil {
		return false
	}

	return c.Permissions.Analyzers.allows(identifiers)
}

// IsResponderAllowed reports whether a responder may run. Pass every identifier
// the responder is known by, its ID and its name, since the rules may list either.
func (c *Config) IsResponderAllowed(identifiers ...string) bool {
	if c == nil {
		return false
	}

	return c.Permissions.Responders.allows(identifiers)
}

// AnalyzerRulesNameWorkers reports whether the analyzer rules single out
// specific analyzers. When they do not, every analyzer gets the same answer,
// so a caller can check one without looking up its other identifier.
func (c *Config) AnalyzerRulesNameWorkers() bool {
	return c != nil && c.Permissions.Analyzers.namesWorkers()
}

// ResponderRulesNameWorkers is AnalyzerRulesNameWorkers for responders.
func (c *Config) ResponderRulesNameWorkers() bool {
	return c != nil && c.Permissions.Responders.namesWorkers()
}

// allows applies the rules to a worker known by several identifiers: an allow
// list permits it when any of them is listed, a block list denies it when any
// of them is blocked. Checking only the identifier a caller sent let a
// responder blocked by name run under its ID.
func (p AutomationPermissions) allows(identifiers []string) bool {
	listed := func(list []string) bool {
		return slices.ContainsFunc(identifiers, func(id string) bool {
			return id != "" && slices.Contains(list, id)
		})
	}

	if !slices.ContainsFunc(identifiers, func(id string) bool { return id != "" }) {
		return false
	}

	switch p.Mode {
	case modeAllowList:
		return slices.Contains(p.Allowed, "*") || listed(p.Allowed)
	case modeBlockList:
		return !listed(p.Blocked)
	default:
		return false
	}
}

// namesWorkers reports whether the answer of allows depends on the worker.
func (p AutomationPermissions) namesWorkers() bool {
	switch p.Mode {
	case modeAllowList:
		return len(p.Allowed) > 0 && !slices.Contains(p.Allowed, "*")
	case modeBlockList:
		return len(p.Blocked) > 0
	default:
		return false
	}
}

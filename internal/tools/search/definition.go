// Package search implements the MCP search tool that queries TheHive entities
// using the TheHive filter DSL.
package search

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// Tool is the MCP tool that searches TheHive entities.
type Tool struct{}

// NewSearchTool returns a new Tool.
func NewSearchTool() *Tool {
	return &Tool{}
}

// Name returns the MCP tool name.
func (t *Tool) Name() string {
	return tools.ToolNameSearchEntities
}

// Handler returns the validated MCP tool handler.
func (t *Tool) Handler() server.ToolHandlerFunc {
	return tools.WithValidation(t)
}

// HasUntrustedData reports that results may contain untrusted user data.
func (t *Tool) HasUntrustedData() bool { return true }

// ValidEntityTypes are the entity types search accepts, in the order the
// advertised enum lists them.
//
// Single source of truth: ValidateParams rejects anything absent from this
// slice and the advertised schema enum is built from it. Two hand-maintained
// lists drifted once already — an entity type accepted by the handler but
// missing from the enum is invisible to clients, which is indistinguishable
// from unsupported.
var ValidEntityTypes = []string{
	types.EntityTypeAlert,
	types.EntityTypeCase,
	types.EntityTypeTask,
	types.EntityTypeObservable,
	types.EntityTypeProcedure,
	types.EntityTypePattern,
	types.EntityTypeCaseTemplate,
	types.EntityTypePage,
	types.EntityTypeJob,
	types.EntityTypeAction,
}

// EntitiesParamConstraints carries the enumerations and defaults that
// EntitiesParams can no longer express as struct tags.
//
// Exported so the schema tests can check every key against the struct.
var EntitiesParamConstraints = map[string]tools.SchemaConstraint{
	"entity-type": {Enum: ValidEntityTypes},
	// job and action default to startDate instead; see types.DefaultSortField.
	// A JSON Schema default is a single value, so it states the general case and
	// the sort-by description carries the exception.
	"sort-by":    {Default: types.GeneralDefaultSortField},
	"sort-order": {Enum: []string{SortOrderAsc, SortOrderDesc}, Default: SortOrderDesc},
	"limit":      {Default: DefaultSearchLimit},
}

// Definition returns the MCP tool definition.
func (t *Tool) Definition() mcp.Tool {
	return mcp.NewTool(
		t.Name(),
		mcp.WithDescription(SearchEntitiesToolDescription),
		tools.WithInputSchemaConstraints[EntitiesParams](EntitiesParamConstraints),
		mcp.WithOutputSchema[EntitiesResult](),
	)
}

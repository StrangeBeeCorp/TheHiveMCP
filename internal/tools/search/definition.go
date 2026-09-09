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
	// sort-by deliberately advertises NO default. The applied default is
	// per-entity-type (types.DefaultSortField: startDate for job and action,
	// _createdAt otherwise) and a JSON Schema default is a single value, so any
	// value here would be a false claim for two of the ten types. Worse, a
	// client that materializes defaults would send _createdAt explicitly and
	// silently defeat the per-type default it was trying to honour. The rule
	// lives in the sort-by description instead, which is what a model reads.
	"sort-order": {Enum: []string{SortOrderAsc, SortOrderDesc}, Default: SortOrderDesc},
	"limit":      {Default: DefaultSearchLimit},
	"offset":     {Default: 0},
}

// Definition returns the MCP tool definition.
func (t *Tool) Definition() mcp.Tool {
	return mcp.NewTool(
		t.Name(),
		mcp.WithDescription(SearchEntitiesToolDescription),
		tools.WithInputSchemaConstraints[EntitiesParams](EntitiesParamConstraints),
		tools.WithResultOutputSchema[EntitiesResult](),
		mcp.WithToolAnnotation(tools.ReadOnlyToolAnnotation()),
	)
}

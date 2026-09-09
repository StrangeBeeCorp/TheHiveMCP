package testutils

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/execute_automation"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/manage"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/resource"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/search"
)

// The MCP spec makes structuredContent conforming to the declared outputSchema
// mandatory, so a client is entitled to reject any result that violates it —
// after the handler has already run, and after any write it made is durable.
//
// Nothing in the Go stack notices: mcp-go's client carries no validation code
// at all, and the server's own output validation is opt-in and off (both in
// GetMCPServer and GetInprocessServer, deliberately, so the harness mirrors a
// deployment). A tool could therefore violate its own contract on every call
// while the whole integration suite stayed green — which is exactly what
// happened in v1.1.0, where three tools advertised a wrapper struct while
// emitting the flattened union.
//
// So validate against real TheHive payloads rather than constructed ones.
// GetInprocessServer enables mcp-go's own output validation, which catches
// every call however a test issues it; this helper adds a clearer diagnostic on
// the calls that come through it. Together they complement
// TestToolSchemas_OutputMatchesAdvertisedSchema, which covers every variant but
// only with synthetic values.
var (
	compiledOutputSchemasOnce sync.Once
	compiledOutputSchemas     map[string]*jsonschema.Schema
	errOutputSchemas          error
)

// advertisedOutputSchemas compiles each tool's advertised output schema once.
//
// The definitions are read from the tool packages rather than from ListTools
// because mcp.ToolOutputSchema cannot represent a schema without a top-level
// `type` — an anyOf union decodes to an empty struct client-side — so the typed
// client view would silently validate against nothing.
func advertisedOutputSchemas() (map[string]*jsonschema.Schema, error) {
	compiledOutputSchemasOnce.Do(func() {
		definitions := []mcp.Tool{
			manage.NewManageTool().Definition(),
			search.NewSearchTool().Definition(),
			execute_automation.NewExecuteAutomationTool().Definition(),
			resource.NewResourceTool(nil).Definition(),
		}

		compiled := make(map[string]*jsonschema.Schema, len(definitions))

		for _, definition := range definitions {
			schema, err := compileOutputSchema(definition)
			if err != nil {
				errOutputSchemas = err

				return
			}

			if schema != nil {
				compiled[definition.Name] = schema
			}
		}

		compiledOutputSchemas = compiled
	})

	return compiledOutputSchemas, errOutputSchemas
}

// compileOutputSchema compiles whatever the tool puts on the wire, returning
// nil when it advertises no output schema at all.
func compileOutputSchema(definition mcp.Tool) (*jsonschema.Schema, error) {
	raw := definition.RawOutputSchema
	if len(raw) == 0 {
		if definition.OutputSchema.Type == "" {
			return nil, nil //nolint:nilnil // "no schema advertised" is not an error here
		}

		encoded, err := json.Marshal(definition.OutputSchema)
		if err != nil {
			return nil, err //nolint:wrapcheck // surfaced verbatim by the caller's require
		}

		raw = encoded
	}

	var document any

	err := json.Unmarshal(raw, &document)
	if err != nil {
		return nil, err //nolint:wrapcheck // surfaced verbatim by the caller's require
	}

	compiler := jsonschema.NewCompiler()

	err = compiler.AddResource("output.json", document)
	if err != nil {
		return nil, err //nolint:wrapcheck // surfaced verbatim by the caller's require
	}

	return compiler.Compile("output.json") //nolint:wrapcheck // surfaced verbatim by the caller's require
}

// RequireConformsToOutputSchema asserts a tool result's structured content
// satisfies the schema its tool advertises.
//
// Error results are exempt: the spec's conformance requirement covers the
// success payload, and mcp-go's own output validation skips IsError results for
// the same reason.
func RequireConformsToOutputSchema(t *testing.T, toolName string, result *mcp.CallToolResult) {
	t.Helper()

	if result == nil || result.IsError {
		return
	}

	schemas, err := advertisedOutputSchemas()
	require.NoError(t, err, "advertised output schemas must compile")

	schema, ok := schemas[toolName]
	if !ok {
		return
	}

	// A declared output schema makes structuredContent mandatory on success, so
	// a missing payload is a violation rather than an exemption. mcp-go's own
	// validator skips nil for backwards compatibility with hand-written
	// schemas; here it would hide a handler that returned nothing.
	require.NotNil(t, result.StructuredContent,
		"%s declares an output schema, so a successful result must carry structured content", toolName)

	// Round-trip so validation sees the JSON the client sees, not Go values.
	encoded, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)

	var decoded any

	require.NoError(t, json.Unmarshal(encoded, &decoded))

	require.NoError(t, schema.Validate(decoded),
		"%s returned structured content that violates the output schema it advertises; "+
			"a conformant client would reject this result even though the handler succeeded", toolName)
}

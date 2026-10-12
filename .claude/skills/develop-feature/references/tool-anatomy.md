# Adding a tool, an operation or a parameter

Use `internal/tools/manage/` as the model to copy.

## Parameters and schemas

- **mcp-go v1 can fail to build a schema without telling you.** It infers the input schema from the params struct through `google/jsonschema-go`, which rejects
  the `jsonschema:"enum=..."` tag. When inference fails, the tool is advertised with **zero parameters** and nothing errors. Unit and integration tests still
  pass, because tests build their own arguments and never read the schema.
  - Put enums and defaults in the tool's `map[string]tools.SchemaConstraint`, next to its definition (for example `manage.EntityParamConstraints`). Apply it
    with `tools.WithInputSchemaConstraints[Params](...)`.
  - `bootstrap/tool_schema_test.go` checks every schema: parameters present, descriptions kept, enums survive, required fields. `tool_constraints_test.go`
    checks that the constraints and defaults match the struct and reach the wire. Extend them when you add a parameter or an enum value.
- **The output schema is a union.** `tools.WithUnionOutputSchema[Result]()` builds the schema from every shape the handler returns. A new result shape must be
  one of those shapes:
  - `bootstrap/tool_output_schema_test.go` checks the schema;
  - in integration tests, `testutils.RequireConformsToOutputSchema` checks real payloads against it.
- **Annotations are required.** Every tool declares one in `definition.go`: `ReadOnlyToolAnnotation`, `MutatingToolAnnotation` or
  `ExternallyActingToolAnnotation` (from `internal/tools/annotations.go`). Record the choice in `docs/reference/tools/annotations.md`.

## A new operation or entity type

- Add it to the enum in the constraints map **and** handle it in `handler.go`. Then add the value to the `expected` map in
  `TestToolSchemas_EnumerationsSurviveInference`. That test lists every allowed value by hand, so it fails until you do.
- Gate it in `ValidatePermissions` through `permissions.Config`, using `IsToolAllowed`, `IsEntityOperationAllowed`, or the analyzer and responder allow-lists.
  The default profile is read-only, so a write that is not gated there is a hole.
- Scope checks: operations on entity IDs resolve scope in one batched `_in` query (`internal/utils/entity_scope.go`). Accept bare IDs and `~`-prefixed IDs
  alike.

## A new tool

- Add its name constant in `internal/tools/tool.go` and register it in `bootstrap/server.go:RegisterToolsToMCPServer`.
- Add the name to `validTools` in `internal/permissions/validation.go`. Without it, a permissions file that names the tool is refused.
- Add it to `embedded/default_permissions.yaml`, the example profiles, and `docs/reference/permissions.md`.
- A tool whose results carry TheHive or Cortex data implements `HasUntrustedData() bool { return true }` (see `references/untrusted-data.md`).

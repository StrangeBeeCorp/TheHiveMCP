package resources

import (
	"context"
	"encoding/json"
	"log/slog"
	"reflect"
	"sync"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

// Filter modes advertised per field in an entity's output schema.
//
// TheHive indexes only some of an entity's attributes, and a filter on an
// unindexed one is accepted and returns zero rows — no error, no warning, just
// an empty result indistinguishable from "nothing matched". On Pattern that is
// 12 of 23 attributes, including the ones a caller actually wants
// (`platforms`, `dataSources`, `detection`), so the most useful filters on the
// MITRE catalogue silently return nothing.
//
// TheHive is not hiding this: /api/v1/describe/<entity> reports an `indexType`
// per attribute. We simply never read it, and an output schema listing every
// returned field reads as the filter vocabulary. So translate indexType into
// what a caller needs to know, rather than passing an Elasticsearch detail on
// and hoping the model infers the consequence.
const (
	// filterModeExact takes the full filter DSL (_eq, _gt, _in, …).
	filterModeExact = "exact"
	// filterModeFulltext is indexed for full-text search only, so a substring
	// match works and an exact-equality filter generally does not.
	filterModeFulltext = "fulltext"
	// filterModeNone is returned but not indexed: filtering or sorting on it
	// yields zero rows rather than an error.
	filterModeNone = "no"
)

// indexTypeToFilterMode maps TheHive's declared index types onto filter modes.
// An index type we do not recognise is deliberately absent: saying nothing
// beats guessing, since the whole point is to stop advertising filters that do
// not work.
// The values come from the SDK's AnyIndexType and BasicIndexType enums, which
// between them define exactly standard, none, fulltext and fulltextOnly.
//
// fulltext maps to exact rather than fulltext: the "Only" in its sibling is
// what distinguishes a field indexed for full text *alone* from one indexed
// both ways, and in practice it is what `title` carries on alert, case and
// task — the most routinely filtered field there is. Inferred from the naming
// and from that usage, not from a controlled test: creating data to prove it
// needs an organisation-scoped account.
var indexTypeToFilterMode = map[string]string{
	string(thehive.ANYINDEXTYPE_STANDARD):      filterModeExact,
	string(thehive.ANYINDEXTYPE_FULLTEXT):      filterModeExact,
	string(thehive.ANYINDEXTYPE_FULLTEXT_ONLY): filterModeFulltext,
	string(thehive.ANYINDEXTYPE_NONE):          filterModeNone,
	// BasicIndexType declares only standard and none, whose values are the same
	// strings, so it needs no entries of its own —
	// TestIndexTypeToFilterMode_CoversEveryDeclaredIndexType asserts that.
}

// describeCache memoises the filter modes per entity type, per deployment.
//
// The key includes the target URL and organisation, not just the entity type:
// in HTTP mode every request may name its own TheHive (types.HiveURLCtxKey /
// HiveOrgCtxKey), and indexType can differ between the server versions we
// support. Keyed on the entity alone, the first request to answer would hand
// its own schema shape to every other instance for the life of the process.
//
// Entities whose describe call fails cache nothing, so a transient failure is
// retried rather than remembered as "no opinion".
var describeCache sync.Map // describeCacheKey -> map[string]string

// describeCacheKey identifies one entity's description on one deployment.
type describeCacheKey struct {
	url          string
	organisation string
	entityType   string
}

// cacheKeyFor builds the cache key from the request's target. Both values are
// empty in stdio mode, where a process serves exactly one deployment, so the
// key degrades to the entity type on its own.
func cacheKeyFor(ctx context.Context, entityType string) describeCacheKey {
	url, _ := ctx.Value(types.HiveURLCtxKey).(string)
	organisation, _ := ctx.Value(types.HiveOrgCtxKey).(string)

	return describeCacheKey{url: url, organisation: organisation, entityType: entityType}
}

// withFilterable wraps a static schema handler so the served schema also says,
// per field, whether it can be filtered on.
//
// It is applied at registration rather than inside each of the schema handlers
// to keep them signature-free and side-effect-free; annotation needs a context
// (for the TheHive client) that a static embedded file does not.
func withFilterable(
	entityType string,
	base func() ([]mcp.ResourceContents, error),
) func(context.Context, mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	return func(ctx context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		contents, err := base()
		if err != nil {
			return nil, err
		}

		return annotateFilterable(ctx, entityType, contents), nil
	}
}

// annotateFilterable merges filter modes into a schema's properties.
//
// Fails open in every direction: an unreachable TheHive, an entity with no
// describe endpoint (case-template and task-log have none), a schema shaped
// unexpectedly — all return the schema untouched. A schema without the
// annotation is the status quo; a schema that cannot be served at all would
// take the entity's documentation down with it.
//
// TheHive 5.5 takes that path wholesale: it answers describe, but omits the
// `cardinality` field thehive4go's generated model requires, so the SDK cannot
// decode the response and no field is annotated. The annotation therefore
// needs 5.6+, and fixing it means fixing the SDK rather than this file. See
// issue #181.
func annotateFilterable(ctx context.Context, entityType string, contents []mcp.ResourceContents) []mcp.ResourceContents {
	modes, err := filterModes(ctx, entityType)
	if err != nil || len(modes) == 0 {
		return contents
	}

	annotated := make([]mcp.ResourceContents, 0, len(contents))

	for _, content := range contents {
		text, isText := content.(mcp.TextResourceContents)
		if !isText {
			annotated = append(annotated, content)

			continue
		}

		merged, mergeErr := mergeFilterModes([]byte(text.Text), modes)
		if mergeErr != nil {
			slog.Debug("Could not annotate schema with filterable fields",
				"entityType", entityType, "error", mergeErr)

			annotated = append(annotated, content)

			continue
		}

		text.Text = string(merged)
		annotated = append(annotated, text)
	}

	return annotated
}

// filterableKey is the per-property key carrying the filter mode.
const filterableKey = "filterable"

// mergeFilterModes writes a filter mode onto each property the description
// covers, and documents the key at the top level so a reader knows what it
// means without being told separately.
//
// A property TheHive says nothing about is left alone rather than defaulted:
// the schemas carry fields describe does not list, and marking those
// unfilterable would trade one wrong answer for another.
func mergeFilterModes(schemaJSON []byte, modes map[string]string) ([]byte, error) {
	var schema map[string]any

	err := json.Unmarshal(schemaJSON, &schema)
	if err != nil {
		return nil, err //nolint:wrapcheck // caller logs and falls back to the unannotated schema
	}

	annotated := annotateProperties(schema, "", modes)
	if annotated == 0 {
		return schemaJSON, nil
	}

	schema[filterableKey+"Legend"] = map[string]any{
		filterModeExact:    "supports the full filter DSL and sorting",
		filterModeFulltext: "indexed for full-text only: substring matches work, exact equality generally does not",
		filterModeNone:     "returned but NOT indexed: a filter or sort on this field yields zero rows, not an error",
		"absent":           "TheHive does not describe this field; filterability unknown",
	}

	return json.Marshal(schema) //nolint:wrapcheck // caller logs and falls back to the unannotated schema
}

// annotateProperties walks a schema, marking every property TheHive describes,
// and returns how many it marked.
//
// It recurses because the two namings do not line up: describe reports
// `importDate` flat while OutputAlert nests it under `extraData`, and reports
// `computed.handlingDuration` dotted. So each property is looked up by its
// dotted path first, then by its bare name.
//
// Matching a bare name at depth accepts a small risk: a nested field sharing a
// top-level attribute's name would inherit its mode. That is worth it — the
// annotation is advisory metadata, and the alternative is missing the
// unindexed fields that motivated this, which are precisely the nested ones.
func annotateProperties(node map[string]any, prefix string, modes map[string]string) int {
	properties, ok := node["properties"].(map[string]any)
	if !ok {
		return annotateItems(node, prefix, modes)
	}

	annotated := 0

	for name, raw := range properties {
		property, isObject := raw.(map[string]any)
		if !isObject {
			continue
		}

		path := name
		if prefix != "" {
			path = prefix + "." + name
		}

		mode, known := modes[path]
		if !known {
			mode, known = modes[name]
		}

		if known {
			property[filterableKey] = mode
			annotated++
		}

		annotated += annotateProperties(property, path, modes)
	}

	return annotated
}

// annotateItems descends into an array's element schema, which carries the
// properties of the objects the array holds.
func annotateItems(node map[string]any, prefix string, modes map[string]string) int {
	items, ok := node["items"].(map[string]any)
	if !ok {
		return 0
	}

	return annotateProperties(items, prefix, modes)
}

// filterModes returns the filter mode of each described attribute of an entity.
func filterModes(ctx context.Context, entityType string) (map[string]string, error) {
	cacheKey := cacheKeyFor(ctx, entityType)

	if cached, hit := describeCache.Load(cacheKey); hit {
		modes, ok := cached.(map[string]string)
		if ok {
			return modes, nil
		}
	}

	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, err //nolint:wrapcheck // annotation is best-effort; the caller falls back
	}

	description, resp, err := hiveClient.DescribeAPI.DescribeAModel(ctx, entityType).Execute()
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}

	if err != nil {
		return nil, err //nolint:wrapcheck // annotation is best-effort; the caller falls back
	}

	modes := make(map[string]string, len(description.Attributes))

	for _, attribute := range description.Attributes {
		name, indexType, ok := attributeIndexType(attribute)
		if !ok {
			continue
		}

		mode, recognised := indexTypeToFilterMode[indexType]
		if !recognised {
			continue
		}

		modes[name] = mode
	}

	describeCache.Store(cacheKey, modes)

	return modes, nil
}

// attributeIndexType pulls the name and index type out of one described
// attribute.
//
// PropertyDescription is a generated oneOf wrapper over eight typed variants
// (string, boolean, date, enumeration, …) that all carry Name and IndexType,
// so read them reflectively off the active variant rather than spelling out an
// eight-way type switch that a ninth variant would silently escape.
func attributeIndexType(attribute thehive.PropertyDescription) (name, indexType string, ok bool) {
	instance := attribute.GetActualInstance()
	if instance == nil {
		return "", "", false
	}

	value := reflect.ValueOf(instance)
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return "", "", false
		}

		value = value.Elem()
	}

	if value.Kind() != reflect.Struct {
		return "", "", false
	}

	nameField := value.FieldByName("Name")
	indexField := value.FieldByName("IndexType")

	if !nameField.IsValid() || !indexField.IsValid() || nameField.Kind() != reflect.String {
		return "", "", false
	}

	return nameField.String(), indexTypeString(indexField), true
}

// indexTypeString renders an index type value, which the generator emits as
// its own named type rather than a bare string.
func indexTypeString(value reflect.Value) string {
	if value.Kind() == reflect.String {
		return value.String()
	}

	if marshaller, isMarshaller := value.Interface().(json.Marshaler); isMarshaller {
		raw, err := marshaller.MarshalJSON()
		if err == nil {
			var unquoted string
			if json.Unmarshal(raw, &unquoted) == nil {
				return unquoted
			}
		}
	}

	return ""
}

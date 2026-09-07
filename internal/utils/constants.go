package utils

// Query pipeline operation and field name literals shared across this package.
// Extracted to constants so the same string is not duplicated (goconst) and to
// give the query DSL vocabulary a single spelling.
const (
	// opNameKey is the TheHive query-operation name key.
	opNameKey = "_name"
	// idOrNameKey is the TheHive get-by-id/name selector key.
	idOrNameKey = "idOrName"

	// opFilter is the TheHive filter operation name.
	opFilter = "filter"
	// opAnd, opIn, keyField, keyValues are query-filter DSL keys used to build the
	// scope filter (_and[ permFilters, _in{_field:_id, _values:ids} ]).
	opAnd     = "_and"
	opIn      = "_in"
	keyField  = "_field"
	keyValues = "_values"

	// scopeBatchChunkSize caps how many ids ride in a single scopedEntityIDsBatch
	// _in{_id} query. A large batch is split into ceil(N/chunk) list queries whose
	// matched sets are unioned, so the _in clause can never exceed a server-side
	// cap and come back silently truncated (which would read as a false "out of
	// scope" — a mass-denial bug, DL-5764). Chosen at 500, comfortably below
	// Elasticsearch's default indices.query.bool.max_clause_count (1024) while
	// still collapsing a realistic multi-parent expansion (hundreds of hits) into
	// one or two round-trips instead of N.
	scopeBatchChunkSize = 500

	// fieldID is the top-level entity identifier field.
	fieldID = "_id"
	// fieldDataType is the observable data-type field.
	fieldDataType = "dataType"
	// fieldHashes is the attachment digests field; fieldTactics the pattern tactics field.
	fieldHashes  = "hashes"
	fieldTactics = "tactics"
	// fieldObservableCount is a similarity match-context meta field.
	fieldObservableCount = "observableCount"

	// rawFiltersKey names the MCP/LLM-generated query-structure subtree that must
	// not be [UNTRUSTED_DATA]-wrapped.
	rawFiltersKey = "rawFilters"

	// querySimilarCases and querySimilarAlerts are the MCP-facing additional-query
	// names for the similarity expansions.
	querySimilarCases  = "similarCases"
	querySimilarAlerts = "similarAlerts"

	// queryActions is the additional-query name for the Cortex responder runs of
	// an entity. Every entity type Cortex can act on supports it.
	queryActions = "actions"
)

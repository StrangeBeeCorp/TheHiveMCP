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

	// fieldID is the top-level entity identifier field.
	fieldID = "_id"
	// fieldDataType is the observable data-type field.
	fieldDataType = "dataType"
	// fieldObservableCount is a similarity match-context meta field.
	fieldObservableCount = "observableCount"

	// rawFiltersKey names the MCP/LLM-generated query-structure subtree that must
	// not be [UNTRUSTED_DATA]-wrapped.
	rawFiltersKey = "rawFilters"

	// querySimilarCases and querySimilarAlerts are the MCP-facing additional-query
	// names for the similarity expansions.
	querySimilarCases  = "similarCases"
	querySimilarAlerts = "similarAlerts"
)

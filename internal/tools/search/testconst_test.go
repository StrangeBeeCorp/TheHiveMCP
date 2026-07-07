package search_test

// Filter DSL keys, entity field names and tool parameter names repeated across
// the search test files. Centralized to satisfy goconst and keep the tests
// consistent.
const (
	tField     = "_field"
	tValue     = "_value"
	tValues    = "_values"
	tID        = "_id"
	tTitle     = "title"
	tSeverity  = "severity"
	tStatus    = "status"
	tTags      = "tags"
	tTasks     = "tasks"
	tCaseID    = "case_id"
	tCount     = "count"
	tCreatedAt = "_createdAt"

	tEq      = "_eq"
	tGte     = "_gte"
	tLte     = "_lte"
	tBetween = "_between"
	tIn      = "_in"
	tLike    = "_like"
	tAnd     = "_and"
	tOr      = "_or"

	tPhishing = "phishing"
	tMalware  = "malware"
	tNetwork  = "network"

	pEntityType        = "entity-type"
	pFilters           = "filters"
	pExtraColumns      = "extra-columns"
	pAdditionalQueries = "additional-queries"
)

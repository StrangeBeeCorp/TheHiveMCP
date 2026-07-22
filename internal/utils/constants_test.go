package utils

// Test-only string literals reused across the package's tests. Extracted so the
// same literal is not repeated (goconst); production literals are reused from
// constants.go where one already exists.
const (
	fieldType     = "_type"
	fieldField    = "_field"
	fieldValue    = "_value"
	fieldTitle    = "title"
	fieldSeverity = "severity"
	fieldStatus   = "status"

	fieldSimilarObservableCount = "similarObservableCount"

	opEq   = "_eq"
	opLike = "_like"
	opLTE  = "_lte"
	opGTE  = "_gte"

	valueCase = "case"
	fieldTLP  = "tlp"
	fieldTags = "tags"

	fieldMessage = "message"
	fieldSummary = "summary"

	valueInitialAccess = "initial-access"

	valueNew = "New"

	opGetCase  = "getCase"
	opGetAlert = "getAlert"

	opListCase  = "listCase"
	opListAlert = "listAlert"

	methodPost      = "POST"
	pathCaseByID    = "/api/v1/case/~1"
	pathQuery       = "/api/v1/query"
	filterPhishing  = "*Phishing*"
	quotedFieldName = `"_field"`
)

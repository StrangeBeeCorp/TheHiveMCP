package search

// Entity field names reused across the search tool.
const (
	fieldID    = "_id"
	fieldTitle = "title"
	// fieldName is the pattern's technique name (and the case template's name).
	fieldName = "name"
	// fieldPlatforms is the pattern's platform list: returned on request, not
	// indexed, so never filterable.
	fieldPlatforms = "platforms"
	// fieldCount is the key a count query's single result row carries.
	fieldCount = "_count"
)

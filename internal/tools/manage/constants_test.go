package manage_test

// Shared string literals used across the manage_test package tests.
// Grouped by role: tool argument keys, operation values, and entity field names.
const (
	// Tool argument keys.
	testToolName      = "manage-entities"
	testArgOperation  = "operation"
	testArgEntityType = "entity-type"
	testArgEntityIDs  = "entity-ids"
	testArgEntityData = "entity-data"
	testArgTargetID   = "target-id"

	// Operation values.
	testOpCreate  = "create"
	testOpUpdate  = "update"
	testOpDelete  = "delete"
	testOpComment = "comment"
	testOpPromote = "promote"
	testOpMerge   = "merge"

	// Entity field names.
	testFieldTitle       = "title"
	testFieldDescription = "description"
	testFieldSeverity    = "severity"
	testFieldTLP         = "tlp"
	testFieldPAP         = "pap"
	testFieldTags        = "tags"
	testFieldType        = "test-type"
	testFieldSource      = "source"
	testFieldSourceRef   = "sourceRef"
	testFieldDataType    = "dataType"
	testFieldData        = "data"
	testFieldMessage     = "message"
	testFieldContent     = "content"
	testFieldTypeKey     = "type"
	testFieldCategory    = "category"

	// Common values.
	testCategoryDefault = "Default"
	testEntityID        = "~123"
	testValueSource     = "test-source"
)

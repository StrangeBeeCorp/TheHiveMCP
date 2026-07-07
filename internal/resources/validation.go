package resources

import (
	"fmt"
	"regexp"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// allowedResponderEntityTypes gates the {entityType} path segment of
// /api/v1/connector/cortex/responders/{entityType}/{entityId}; anything else is
// rejected before reaching TheHive (RandoriSec 5.3, DL-6004).
var allowedResponderEntityTypes = map[string]bool{
	types.EntityTypeCase:       true,
	types.EntityTypeAlert:      true,
	types.EntityTypeTask:       true,
	types.EntityTypeObservable: true,
	"case_artifact":            true,
	"log":                      true,
}

// entityIDPattern matches a TheHive entity ID: "~" + digits (JanusGraph vertex
// ID, variable length). Shape has no path separators/dots/whitespace, so
// traversal sequences like "../" or "%2e%2e" cannot match.
var entityIDPattern = regexp.MustCompile(`^~[0-9]+$`)

// validateResponderParams rejects entityType/entityId values that could escape
// the responder endpoint's path boundary. Must run before any Cortex call.
func validateResponderParams(entityType, entityID string) error {
	if !allowedResponderEntityTypes[entityType] {
		return fmt.Errorf("invalid entityType %q: must be one of case, alert, task, observable, case_artifact, log", entityType)
	}

	if !entityIDPattern.MatchString(entityID) {
		return fmt.Errorf("invalid entityId %q: must be a TheHive entity ID of the form '~' followed by digits, e.g. ~123456", entityID)
	}

	return nil
}

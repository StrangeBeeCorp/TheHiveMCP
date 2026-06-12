package resources

import (
	"fmt"
	"regexp"
)

// allowedResponderEntityTypes is the allowlist of entity types accepted by
// TheHive's Cortex responder endpoint
// (/api/v1/connector/cortex/responders/{entityType}/{entityId}).
// Anything else is rejected before reaching TheHive (RandoriSec 5.3, DL-6004).
var allowedResponderEntityTypes = map[string]bool{
	"case":          true,
	"alert":         true,
	"task":          true,
	"observable":    true,
	"case_artifact": true,
	"log":           true,
}

// entityIDPattern matches TheHive entity identifiers: an optional "~" prefix
// followed by an alphanumeric token (with "-" and "_"). IDs never contain
// path separators, dots, fragments, or whitespace, so traversal sequences
// such as "../" or "%2e%2e" cannot match.
var entityIDPattern = regexp.MustCompile(`^~?[A-Za-z0-9_-]+$`)

// validateResponderParams rejects entityType/entityId values that could
// escape the responder endpoint's path boundary. It must run before any
// Cortex call is made.
func validateResponderParams(entityType, entityID string) error {
	if !allowedResponderEntityTypes[entityType] {
		return fmt.Errorf("invalid entityType %q: must be one of case, alert, task, observable, case_artifact, log", entityType)
	}
	if !entityIDPattern.MatchString(entityID) {
		return fmt.Errorf("invalid entityId %q: must be a TheHive entity ID containing only letters, digits, '-' or '_', optionally prefixed with '~', e.g. ~123456", entityID)
	}
	return nil
}

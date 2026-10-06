package tools

import "strings"

// ValidatePathIDs rejects an ID that could leave the URL path segment it is
// interpolated into. The generated client percent-encodes every path value, so
// "/" cannot split a segment on its own, but a proxy that decodes %2F before
// routing would split it, a dot segment ("." or "..") is not encoded at all,
// some servers treat "\" as "/", and an empty ID turns an entity's path into
// its collection's. The class RandoriSec 5.3 found open to traversal.
//
// It is the floor for IDs that may also be names (a case template, a page):
// an ID known to be TheHive's "~" + digits is held to that pattern instead.
func ValidatePathIDs(param string, ids ...string) *ToolError {
	for _, id := range ids {
		if !IsPathSegment(id) {
			return NewToolErrorf("invalid %s %q: an ID cannot be empty, a dot segment, or contain a slash, a backslash or a control character", param, id).
				Hint("Use the entity's _id (such as ~123456) or its name exactly as TheHive returns it")
		}
	}

	return nil
}

// IsPathSegment reports whether value can be one URL path segment: not empty,
// not a dot segment, and free of slashes, backslashes and control characters.
func IsPathSegment(value string) bool {
	return value != "" && value != "." && value != ".." &&
		!strings.ContainsAny(value, `/\`) && !strings.ContainsFunc(value, isControl)
}

func isControl(r rune) bool {
	return r < 0x20 || r == 0x7f
}

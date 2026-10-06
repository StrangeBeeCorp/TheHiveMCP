package tools

import "strings"

// ValidatePathIDs rejects an ID that could leave the URL path segment it is
// interpolated into. The generated client percent-encodes every path value, so
// "/" cannot split a segment on its own, but a proxy that decodes %2F before
// routing would split it, a dot segment ("." or "..") is not encoded at all,
// and some servers treat "\" as "/". The class RandoriSec 5.3 found open to
// traversal.
//
// It is the floor for IDs that may also be names (a case template, a page):
// an ID known to be TheHive's "~" + digits is held to that pattern instead.
func ValidatePathIDs(param string, ids ...string) *ToolError {
	for _, id := range ids {
		if id == "." || id == ".." || strings.ContainsAny(id, `/\`) || strings.ContainsFunc(id, isControl) {
			return NewToolErrorf("invalid %s %q: an ID cannot be a dot segment or contain a slash, a backslash or a control character", param, id).
				Hint("Use the entity's _id (such as ~123456) or its name exactly as TheHive returns it")
		}
	}

	return nil
}

func isControl(r rune) bool {
	return r < 0x20 || r == 0x7f
}

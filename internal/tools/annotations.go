package tools

import "github.com/mark3labs/mcp-go/mcp"

// Tool annotations are the behavioural hints a client reads to decide how much
// ceremony a call deserves — whether to prompt before running it, whether it may
// be auto-approved, whether a retry is safe.
//
// Setting them is not optional in practice. mcp.NewTool populates Annotations
// itself and the field carries no `omitempty`, so a tool that declares nothing
// still advertises mcp-go's deliberately pessimistic defaults:
//
//	{"readOnlyHint":false,"destructiveHint":true,"idempotentHint":false,"openWorldHint":true}
//
// Every tool here shipped exactly that, which claimed search-entities and
// get-resource may perform destructive updates. Silence is a wrong answer, not
// an absent one.
//
// The hints describe what the TOOL can do, not what a given deployment permits.
// All four tools are registered unconditionally and permissions are enforced per
// call in ValidatePermissions, so a read-only deployment still advertises
// manage-entities as mutating. That is the correct reading: the MCP spec treats
// annotations as untrusted hints from the server, never as an authorization
// boundary.

// ReadOnlyToolAnnotation describes a tool that only ever reads.
//
// destructiveHint and idempotentHint are meaningless while readOnlyHint is
// true, but mcp-go emits all four keys regardless, so they are set to the values
// that are trivially true of a read rather than left at the pessimistic default.
func ReadOnlyToolAnnotation() mcp.ToolAnnotation {
	return mcp.ToolAnnotation{
		ReadOnlyHint:    new(true),
		DestructiveHint: new(false),
		IdempotentHint:  new(true),
		OpenWorldHint:   new(false),
	}
}

// MutatingToolAnnotation describes a tool that changes state inside TheHive and
// nowhere else.
//
// destructive because the set of operations includes irreversible ones (delete,
// merge) — the hint covers what the tool can do, not what a particular call
// does. Not idempotent: a repeated create makes a second entity.
func MutatingToolAnnotation() mcp.ToolAnnotation {
	return mcp.ToolAnnotation{
		ReadOnlyHint:    new(false),
		DestructiveHint: new(true),
		IdempotentHint:  new(false),
		OpenWorldHint:   new(false),
	}
}

// ExternallyActingToolAnnotation describes a tool whose effects escape TheHive.
//
// This is the only openWorldHint:true tool. The others reach exactly one known,
// configured system, which is a closed domain however remote it is; Cortex
// analyzers and responders reach an open-ended set of third-party services —
// submitting an observable to a sandbox, or having a responder block an address
// — that the server cannot enumerate and cannot undo.
func ExternallyActingToolAnnotation() mcp.ToolAnnotation {
	return mcp.ToolAnnotation{
		ReadOnlyHint:    new(false),
		DestructiveHint: new(true),
		IdempotentHint:  new(false),
		OpenWorldHint:   new(true),
	}
}

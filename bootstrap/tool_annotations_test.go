package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mcp.NewTool fills Annotations in itself and the field carries no `omitempty`,
// so a tool that sets nothing still ships mcp-go's pessimistic defaults —
// readOnlyHint:false, destructiveHint:true. That is what all four tools
// advertised before these were declared, telling clients that search-entities
// and get-resource may perform destructive updates.
//
// The failure mode is quiet in both directions: forgetting the annotation on a
// new tool looks like nothing, and a copy-pasted definition can hand a mutating
// tool the read-only profile — which clients read as safe to auto-approve. Both
// are caught only by pinning the exact hints.
func TestToolAnnotations_MatchWhatEachToolCanDo(t *testing.T) {
	t.Parallel()

	type hints struct {
		readOnly, destructive, idempotent, openWorld bool
	}

	expected := map[string]hints{
		// Reads TheHive; cannot change anything.
		toolSearchEntities: {readOnly: true, destructive: false, idempotent: true, openWorld: false},
		// Serves schemas and docs from the server's own registry.
		toolGetResource: {readOnly: true, destructive: false, idempotent: true, openWorld: false},
		// create/update/delete/merge inside TheHive: mutating and irreversible,
		// but reaching no further than the one configured server.
		toolManageEntities: {readOnly: false, destructive: true, idempotent: false, openWorld: false},
		// Cortex analyzers and responders act on third-party services the server
		// cannot enumerate, which is what openWorldHint is for.
		toolExecuteAutomation: {readOnly: false, destructive: true, idempotent: false, openWorld: true},
	}

	subjects := toolsUnderTest()
	require.Len(t, expected, len(subjects), "every registered tool must pin its annotations")

	for name, want := range expected {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			subject, registered := subjects[name]
			require.True(t, registered, "no registered tool named %q", name)

			annotations := subject.tool.Annotations

			require.NotNil(t, annotations.ReadOnlyHint, "tool %q advertises no readOnlyHint", name)
			require.NotNil(t, annotations.DestructiveHint, "tool %q advertises no destructiveHint", name)
			require.NotNil(t, annotations.IdempotentHint, "tool %q advertises no idempotentHint", name)
			require.NotNil(t, annotations.OpenWorldHint, "tool %q advertises no openWorldHint", name)

			assert.Equal(t, want.readOnly, *annotations.ReadOnlyHint, "tool %q readOnlyHint", name)
			assert.Equal(t, want.destructive, *annotations.DestructiveHint, "tool %q destructiveHint", name)
			assert.Equal(t, want.idempotent, *annotations.IdempotentHint, "tool %q idempotentHint", name)
			assert.Equal(t, want.openWorld, *annotations.OpenWorldHint, "tool %q openWorldHint", name)
		})
	}
}

// A read-only tool that reaches TheHive is still read-only, so the hints must
// never be inferred from "does it make a network call". Guard the invariant that
// actually matters to a client: nothing claiming readOnlyHint may also claim it
// destroys, and every mutating tool must be flagged destructive, since none of
// them is limited to reversible operations.
func TestToolAnnotations_ReadOnlyAndDestructiveAgree(t *testing.T) {
	t.Parallel()

	for name, subject := range toolsUnderTest() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			annotations := subject.tool.Annotations
			require.NotNil(t, annotations.ReadOnlyHint)
			require.NotNil(t, annotations.DestructiveHint)

			assert.Equal(t, !*annotations.ReadOnlyHint, *annotations.DestructiveHint,
				"tool %q: a read-only tool cannot be destructive, and every mutating tool here can delete or act irreversibly", name)
		})
	}
}

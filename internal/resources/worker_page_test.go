package resources

import (
	"context"
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// notANumber is an offset/limit value that must fail to parse.
const notANumber = "many"

func workers(count int) []thehive.OutputWorker {
	list := make([]thehive.OutputWorker, 0, count)
	for i := range count {
		list = append(list, thehive.OutputWorker{Id: string(rune('a' + i)), Name: string(rune('a' + i))})
	}

	return list
}

func requestWithArguments(arguments map[string]any) mcp.ReadResourceRequest {
	return mcp.ReadResourceRequest{Params: mcp.ReadResourceParams{Arguments: arguments}}
}

func TestNewWorkerPageReportsTruncation(t *testing.T) {
	t.Parallel()

	page := newWorkerPage("analyzers", workers(10), 0, 0, 4)

	assert.Equal(t, 10, page.Total)
	assert.Equal(t, 4, page.Returned)
	assert.True(t, page.Truncated, "a window smaller than the catalog must be reported as truncated")
	assert.Len(t, page.Workers, 4)
}

func TestNewWorkerPageCompleteCatalogIsNotTruncated(t *testing.T) {
	t.Parallel()

	page := newWorkerPage("analyzers", workers(3), 0, 0, 50)

	assert.Equal(t, 3, page.Total)
	assert.Equal(t, 3, page.Returned)
	assert.False(t, page.Truncated)
}

func TestNewWorkerPageOffsetWindow(t *testing.T) {
	t.Parallel()

	page := newWorkerPage("analyzers", workers(5), 0, 3, 2)

	assert.Equal(t, 3, page.Offset)
	assert.Equal(t, 2, page.Returned)
	assert.False(t, page.Truncated, "the window reaching the end of the catalog is not truncated")
	assert.Equal(t, "d", page.Workers[0].GetId())
}

// An offset past the end must come back as an empty page rather than panicking
// on the slice bounds.
func TestNewWorkerPageOffsetBeyondCatalog(t *testing.T) {
	t.Parallel()

	page := newWorkerPage("responders", workers(2), 0, 99, 10)

	assert.Equal(t, 2, page.Total)
	assert.Equal(t, 0, page.Returned)
	assert.Equal(t, 2, page.Offset)
	assert.Empty(t, page.Workers)
}

func TestNewWorkerPageEmptyCatalogSerializesAsList(t *testing.T) {
	t.Parallel()

	page := newWorkerPage("analyzers", nil, 0, 0, 50)

	assert.NotNil(t, page.Workers, "workers must marshal as [] rather than null")
	assert.Empty(t, page.Workers)
}

// An empty catalog caused by an allow-list must be distinguishable from a
// deployment that genuinely has no analyzers: the shipped read-only default
// blocks every analyzer, and silence there reads as "Cortex is not connected".
func TestNewWorkerPageReportsPolicyBlockedCount(t *testing.T) {
	t.Parallel()

	page := newWorkerPage("analyzers", nil, 29, 0, 50)

	assert.Equal(t, 0, page.Total)
	assert.Equal(t, 29, page.BlockedByPolicy)
	assert.Empty(t, page.Workers)
}

func TestRejectUnknownArguments(t *testing.T) {
	t.Parallel()

	// A responder-shaped call against the analyzer catalog is the likeliest wrong
	// guess; ignoring it would return the whole catalog as if it were filtered.
	err := rejectUnknownArguments(requestWithArguments(map[string]any{"entityType": "observable"}), argDataType, argOffset, argLimit)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown query parameter "entityType"`)
	assert.Contains(t, err.Error(), argDataType)

	// A misspelled known parameter is rejected rather than silently ignored.
	require.Error(t, rejectUnknownArguments(requestWithArguments(map[string]any{"datatype": "ip"}), argDataType))

	require.NoError(t, rejectUnknownArguments(requestWithArguments(map[string]any{argDataType: "ip", argLimit: "5"}), argDataType, argOffset, argLimit))
	require.NoError(t, rejectUnknownArguments(requestWithArguments(nil), argDataType))
}

func TestDataTypeArgument(t *testing.T) {
	t.Parallel()

	value, err := dataTypeArgument(requestWithArguments(map[string]any{argDataType: "hash"}))
	require.NoError(t, err)
	assert.Equal(t, "hash", value)

	// Absent is fine — it means "whole catalog".
	value, err = dataTypeArgument(requestWithArguments(nil))
	require.NoError(t, err)
	assert.Empty(t, value)

	// Present but blank is not: it would silently widen the answer.
	_, err = dataTypeArgument(requestWithArguments(map[string]any{argDataType: "  "}))
	require.Error(t, err)
}

func TestPaginationArgumentsDefaults(t *testing.T) {
	t.Parallel()

	offset, limit, err := paginationArguments(requestWithArguments(nil))

	require.NoError(t, err)
	assert.Equal(t, 0, offset)
	assert.Equal(t, defaultWorkerPageLimit, limit)
}

func TestPaginationArgumentsParsesStrings(t *testing.T) {
	t.Parallel()

	offset, limit, err := paginationArguments(requestWithArguments(map[string]any{argOffset: "10", argLimit: "5"}))

	require.NoError(t, err)
	assert.Equal(t, 10, offset)
	assert.Equal(t, 5, limit)
}

func TestPaginationArgumentsCapsLimit(t *testing.T) {
	t.Parallel()

	_, limit, err := paginationArguments(requestWithArguments(map[string]any{argLimit: "100000"}))

	require.NoError(t, err)
	assert.Equal(t, maxWorkerPageLimit, limit)
}

func TestPaginationArgumentsRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	testCases := map[string]map[string]any{
		"non numeric limit": {argLimit: notANumber},
		"negative offset":   {argOffset: "-1"},
		"zero limit":        {argLimit: "0"},
	}

	for name, arguments := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, _, err := paginationArguments(requestWithArguments(arguments))
			require.Error(t, err)
		})
	}
}

func TestStringArgumentMissingIsEmpty(t *testing.T) {
	t.Parallel()

	assert.Empty(t, stringArgument(requestWithArguments(nil), argDataType))
	assert.Equal(t, "hash", stringArgument(requestWithArguments(map[string]any{argDataType: "hash"}), argDataType))
}

// An invalid offset/limit must be rejected without contacting Cortex. Both
// catalog handlers fetch the whole catalog (range=all for analyzers), so
// validating the window after the fetch charged a deterministic input error a
// full round-trip. No TheHive client is placed in the context here: reaching
// the fetch would fail with a client error instead of the argument error, so
// the assertion on the message is what pins the ordering.
func TestCatalogsRejectBadPagingBeforeFetching(t *testing.T) {
	t.Parallel()

	testCases := map[string]map[string]any{
		"analyzers non numeric limit": {argLimit: notANumber},
		"analyzers negative offset":   {argOffset: "-1"},
	}

	for name, arguments := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := GetAvailableAnalyzers(context.Background(), requestWithArguments(arguments))
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "TheHive client",
				"paging was validated after the fetch: the request reached the client")
		})
	}

	t.Run("responders bad limit", func(t *testing.T) {
		t.Parallel()

		_, err := GetAvailableResponders(context.Background(), requestWithArguments(map[string]any{
			argEntityType: "case",
			argEntityID:   "~123456",
			argLimit:      notANumber,
		}))
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "TheHive client",
			"paging was validated after the fetch: the request reached the client")
	})
}

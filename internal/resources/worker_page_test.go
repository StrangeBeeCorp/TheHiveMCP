package resources

import (
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	argOffset = "offset"
	argLimit  = "limit"
)

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

	page := newWorkerPage("analyzers", workers(10), 0, 4)

	assert.Equal(t, 10, page.Total)
	assert.Equal(t, 4, page.Returned)
	assert.True(t, page.Truncated, "a window smaller than the catalog must be reported as truncated")
	assert.Len(t, page.Workers, 4)
}

func TestNewWorkerPageCompleteCatalogIsNotTruncated(t *testing.T) {
	t.Parallel()

	page := newWorkerPage("analyzers", workers(3), 0, 50)

	assert.Equal(t, 3, page.Total)
	assert.Equal(t, 3, page.Returned)
	assert.False(t, page.Truncated)
}

func TestNewWorkerPageOffsetWindow(t *testing.T) {
	t.Parallel()

	page := newWorkerPage("analyzers", workers(5), 3, 2)

	assert.Equal(t, 3, page.Offset)
	assert.Equal(t, 2, page.Returned)
	assert.False(t, page.Truncated, "the window reaching the end of the catalog is not truncated")
	assert.Equal(t, "d", page.Workers[0].GetId())
}

// An offset past the end must come back as an empty page rather than panicking
// on the slice bounds.
func TestNewWorkerPageOffsetBeyondCatalog(t *testing.T) {
	t.Parallel()

	page := newWorkerPage("responders", workers(2), 99, 10)

	assert.Equal(t, 2, page.Total)
	assert.Equal(t, 0, page.Returned)
	assert.Equal(t, 2, page.Offset)
	assert.Empty(t, page.Workers)
}

func TestNewWorkerPageEmptyCatalogSerializesAsList(t *testing.T) {
	t.Parallel()

	page := newWorkerPage("analyzers", nil, 0, 50)

	assert.NotNil(t, page.Workers, "workers must marshal as [] rather than null")
	assert.Empty(t, page.Workers)
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
		"non numeric limit": {argLimit: "many"},
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

	assert.Empty(t, stringArgument(requestWithArguments(nil), "dataType"))
	assert.Equal(t, "hash", stringArgument(requestWithArguments(map[string]any{"dataType": "hash"}), "dataType"))
}

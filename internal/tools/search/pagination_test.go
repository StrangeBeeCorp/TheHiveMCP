package search

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

func rows(n int) []map[string]any {
	out := make([]map[string]any, n)
	for i := range out {
		out[i] = map[string]any{fieldID: "~" + string(rune('a'+i))}
	}

	return out
}

// A full page and a truncated one arrive as the same row count from TheHive,
// which returns no total. The extra row buildPagingOperation asks for is the
// only thing that tells them apart.
func TestTrimProbeRow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		fetched     int
		limit       int
		wantKept    int
		wantHasMore bool
	}{
		{"short page ends the match", 3, 10, 3, false},
		{"empty page ends the match", 0, 10, 0, false},
		{"exactly full, probe absent", 10, 10, 10, false},
		{"probe present, page truncated", 11, 10, 10, true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			kept, hasMore := trimProbeRow(rows(testCase.fetched), testCase.limit)

			assert.Len(t, kept, testCase.wantKept, "the probe row must never reach the caller")
			assert.Equal(t, testCase.wantHasMore, hasMore)
		})
	}
}

// TheHive's paging bounds are absolute, so offset shifts both ends. The window
// is one row wider than the limit to carry the probe row.
func TestBuildPagingOperation_WindowsTheProbeRow(t *testing.T) {
	t.Parallel()

	tool := &Tool{}

	t.Run("first page", func(t *testing.T) {
		t.Parallel()

		page := tool.buildPagingOperation(0, 10, nil)

		assert.Equal(t, int32(0), page.From)
		assert.Equal(t, int32(11), page.To, "one row past the limit detects truncation")
	})

	t.Run("later page", func(t *testing.T) {
		t.Parallel()

		page := tool.buildPagingOperation(20, 10, nil)

		assert.Equal(t, int32(20), page.From)
		assert.Equal(t, int32(31), page.To)
	})
}

// nextOffset has to land the caller on the row after the page, so that
// re-issuing the same search with it returns the continuation and not a
// re-read of rows already seen.
func TestNewSearchEntitiesResult_Pagination(t *testing.T) {
	t.Parallel()

	params := EntitiesParams{EntityType: types.EntityTypeAlert, Offset: 20, Limit: 10}

	t.Run("truncated page advertises the next window", func(t *testing.T) {
		t.Parallel()

		result, err := NewSearchEntitiesResult(rows(10), params, map[string]any{}, true)
		require.NoError(t, err)

		assert.Equal(t, 10, result.Count, "count is the page size, never the size of the match")
		assert.Equal(t, 20, result.Offset)
		assert.True(t, result.HasMore)
		assert.Equal(t, 30, result.NextOffset)
	})

	t.Run("last page advertises no next window", func(t *testing.T) {
		t.Parallel()

		result, err := NewSearchEntitiesResult(rows(4), params, map[string]any{}, false)
		require.NoError(t, err)

		assert.False(t, result.HasMore)
		assert.Zero(t, result.NextOffset, "nextOffset is omitted from the payload when there is nothing to fetch")
	})

	// count=true aggregates server-side; there is no window, so echoing the
	// caller's offset would describe a page that was never read.
	t.Run("count-only reports no window", func(t *testing.T) {
		t.Parallel()

		countParams := params
		countParams.Count = true

		result, err := NewSearchEntitiesResult([]map[string]any{{"_count": float64(4711)}}, countParams, map[string]any{}, false)
		require.NoError(t, err)

		assert.Equal(t, 4711, result.Count)
		assert.True(t, result.CountOnly)
		assert.Zero(t, result.Offset)
		assert.False(t, result.HasMore)
		assert.Zero(t, result.NextOffset)
	})
}

func TestValidateParams_Offset(t *testing.T) {
	t.Parallel()

	tool := &Tool{}

	t.Run("defaults to the first page", func(t *testing.T) {
		t.Parallel()

		params := EntitiesParams{EntityType: types.EntityTypeAlert}
		require.NoError(t, tool.ValidateParams(&params))
		assert.Zero(t, params.Offset)
	})

	t.Run("rejects a negative offset", func(t *testing.T) {
		t.Parallel()

		params := EntitiesParams{EntityType: types.EntityTypeAlert, Offset: -1}
		assert.Error(t, tool.ValidateParams(&params))
	})

	// Past index.max_result_window Elasticsearch fails the query itself, so the
	// bound is enforced here where the error can say what to do instead.
	t.Run("rejects paging past the result window", func(t *testing.T) {
		t.Parallel()

		params := EntitiesParams{EntityType: types.EntityTypeAlert, Offset: MaxSearchOffset + 1}
		err := tool.ValidateParams(&params)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Narrow the filter")
	})
}

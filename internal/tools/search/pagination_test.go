package search

import (
	"math"
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

	// TheHive rejects the whole query for exceeding max_result_window, so the
	// probe row is dropped at the boundary rather than pushing past it. Without
	// the clamp the final page is unservable and its rows unreachable, for the
	// sake of a hasMore that can only be false there.
	t.Run("probe row is clamped to the result window", func(t *testing.T) {
		t.Parallel()

		page := tool.buildPagingOperation(MaxSearchWindow-10, 10, nil)

		assert.Equal(t, int32(MaxSearchWindow-10), page.From)
		assert.Equal(t, int32(MaxSearchWindow), page.To, "the window caps the probe, and hasMore is false there")
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

		result, err := NewSearchEntitiesResult([]map[string]any{{fieldCount: float64(4711)}}, countParams, map[string]any{}, false)
		require.NoError(t, err)

		assert.Equal(t, 4711, result.Count)
		assert.True(t, result.CountOnly)
		assert.Zero(t, result.Offset)
		assert.False(t, result.HasMore)
		assert.Zero(t, result.NextOffset)
	})

	// The handler leaves hasMore false on the count path, so this guards the
	// contract rather than a live bug: a count-only result describes no window,
	// and must not advertise one even if a future call site says otherwise.
	t.Run("count-only ignores a stray hasMore", func(t *testing.T) {
		t.Parallel()

		countParams := params
		countParams.Count = true

		result, err := NewSearchEntitiesResult([]map[string]any{{fieldCount: float64(4711)}}, countParams, map[string]any{}, true)
		require.NoError(t, err)

		assert.False(t, result.HasMore, "a count-only result has no window to continue")
		assert.Zero(t, result.NextOffset)
		assert.Zero(t, result.Offset)
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

	// A count reads no window at all, so bounding it rejected count=true with a
	// far offset and told the caller to use count=true — the thing they had done.
	t.Run("count-only is exempt from the window bound", func(t *testing.T) {
		t.Parallel()

		params := EntitiesParams{
			EntityType: types.EntityTypeAlert,
			Count:      true,
			Offset:     MaxSearchWindow - 5,
			Limit:      10,
		}

		require.NoError(t, tool.ValidateParams(&params))
	})

	// The bound covers the window, not the offset: bounding the offset alone let
	// offset=10000 limit=10 through to an opaque server-side failure, and let a
	// large limit blow the window far below that. The page may end exactly on
	// the last readable row — only a page reaching beyond it is refused.
	t.Run("rejects a window past the result limit", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name          string
			offset, limit int
			wantRejected  bool
		}{
			{"a page comfortably inside the window", MaxSearchWindow - 11, 10, false},
			// The probe row would sit one past the window here, so it is clamped
			// away rather than the page being refused: the last readable row stays
			// reachable, and hasMore can only be false there anyway.
			{"page ending exactly on the last readable row", MaxSearchWindow - 10, 10, false},
			{"large limit filling the window exactly", MaxSearchWindow - 1000, 1000, false},
			{"one row too far", MaxSearchWindow - 9, 10, true},
			{"offset alone under the bound, window over it", MaxSearchWindow, 10, true},
			{"large limit blows the window past the cap", MaxSearchWindow - 999, 1000, true},
			{"a near-maxint offset cannot overflow past the check", math.MaxInt, 10, true},
		}

		for _, testCase := range tests {
			t.Run(testCase.name, func(t *testing.T) {
				t.Parallel()

				params := EntitiesParams{
					EntityType: types.EntityTypeAlert,
					Offset:     testCase.offset,
					Limit:      testCase.limit,
				}

				err := tool.ValidateParams(&params)
				if !testCase.wantRejected {
					require.NoError(t, err)

					return
				}

				require.Error(t, err)
				assert.Contains(t, err.Error(), "maximum result window")
			})
		}
	})
}

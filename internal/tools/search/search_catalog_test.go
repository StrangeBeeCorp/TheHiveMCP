package search_test

import (
	"fmt"
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

func TestSearchCaseTemplates(t *testing.T) {
	testutils.Parallel(t)

	hiveClient := testutils.SetupTestWithCleanup(t)

	authContext := testutils.GetAuthContext(t)

	for _, name := range []string{"Phishing-Search-Test", "Malware-Search-Test"} {
		input := testutils.MockInputCaseTemplate()
		input.Name = name
		_, _, err := hiveClient.CaseTemplateAPI.CreateCaseTemplate(authContext).InputCreateCaseTemplate(*input).Execute()
		require.NoError(t, err)
	}

	mcpClient := newSearchClient(t)

	templatesData := searchRows(t, mcpClient, map[string]any{
		pEntityType: types.EntityTypeCaseTemplate,
		pFilters: map[string]any{
			tLike: map[string]any{
				tField: tName,
				tValue: "Phishing*",
			},
		},
		pExtraColumns: []string{tID, tName, "displayName"},
	})
	require.Len(t, templatesData, 1)

	template, ok := templatesData[0].(map[string]any)
	require.True(t, ok)
	// DL-6006: "name" is free text, so it is wrapped (the agent uses tID).
	require.Equal(t, "[UNTRUSTED_DATA]Phishing-Search-Test[/UNTRUSTED_DATA]", template[tName])
}

func TestSearchPages(t *testing.T) {
	testutils.Parallel(t)

	hiveClient := testutils.SetupTestWithCleanup(t)

	authContext := testutils.GetAuthContext(t)
	testCase := testutils.MockInputCase()
	testCase.Title = "Case with Pages for Search"

	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdCase)

	inputPage := thehive.InputCreatePage{
		Title:    "Searchable Investigation Page",
		Content:  "## Notes\nSome investigation content.",
		Category: "Default",
	}
	_, _, err = hiveClient.PageAPI.CreateAPageInACase(authContext, createdCase.UnderscoreId).InputCreatePage(inputPage).Execute()
	require.NoError(t, err)

	mcpClient := newSearchClient(t)

	pagesData := searchRows(t, mcpClient, map[string]any{
		pEntityType: types.EntityTypePage,
		pFilters: map[string]any{
			tEq: map[string]any{
				tField: "category",
				tValue: "Default",
			},
		},
		pExtraColumns: []string{tID, tTitle, "category"},
	})
	require.GreaterOrEqual(t, len(pagesData), 1)

	page, ok := pagesData[0].(map[string]any)
	require.True(t, ok)
	// DL-6006: "category" is a user-defined label, so it is wrapped.
	require.Equal(t, "[UNTRUSTED_DATA]Default[/UNTRUSTED_DATA]", page["category"])
}

// Paging is the whole point of hasMore: a page that comes back exactly full is
// indistinguishable from the end of a match unless the response says so. This
// walks a known 5-alert set two rows at a time and checks both that the flag
// flips on the last page and that following nextOffset actually reaches every
// row exactly once — an off-by-one in the window would re-serve or skip rows
// while still looking like a well-formed page.
func TestSearchPagesThroughResultsWithHasMore(t *testing.T) {
	testutils.Parallel(t)

	hiveClient := testutils.SetupTestWithCleanup(t)

	const (
		total   = 5
		perPage = 2
	)

	// The shared instance carries alerts from other tests, so the filter is
	// scoped to a tag only this test creates.
	pagingTag := "paging-" + t.Name()

	for i := 1; i <= total; i++ {
		createTestAlert(t, hiveClient, fmt.Sprintf("%s alert %d", t.Name(), i), 2, []string{pagingTag})
	}

	mcpClient := newSearchClient(t)

	page := func(offset int) map[string]any {
		return searchStructured(t, mcpClient, map[string]any{
			pEntityType: types.EntityTypeAlert,
			pFilters: map[string]any{
				tEq: map[string]any{tField: tTags, tValue: pagingTag},
			},
			pExtraColumns: []string{tID, tTitle},
			pSortBy:       tCreatedAt,
			pSortOrder:    "asc",
			pLimit:        perPage,
			pOffset:       offset,
		})
	}

	seen := map[string]bool{}
	offset := 0

	for pageNumber := 1; ; pageNumber++ {
		require.LessOrEqual(t, pageNumber, total, "paging did not terminate")

		data := page(offset)
		require.InDelta(t, float64(offset), data[rOffset], 0, "the response echoes the window it read")

		rows, ok := data["results"].([]any)
		require.True(t, ok)

		for _, row := range rows {
			alert, isObject := row.(map[string]any)
			require.True(t, isObject)

			id, isString := alert[tID].(string)
			require.True(t, isString)
			require.False(t, seen[id], "row %s served on more than one page", id)
			seen[id] = true
		}

		hasMore, ok := data[rHasMore].(bool)
		require.True(t, ok, "every search result must state whether it was truncated")

		if !hasMore {
			require.Len(t, rows, total%perPage, "the last page holds the remainder")
			require.NotContains(t, data, rNextOffset, "a complete result advertises no next page")

			break
		}

		require.Len(t, rows, perPage, "a truncated page is full by definition")
		require.InDelta(t, float64(offset+perPage), data[rNextOffset], 0)

		nextOffset, isNumber := data[rNextOffset].(float64)
		require.True(t, isNumber)

		offset = int(nextOffset)
	}

	require.Len(t, seen, total, "following nextOffset must reach every matching row")
}

// TestSearchPatternsCarryNoUntrustedFence pins the trust classification of the
// MITRE ATT&CK catalog end to end: it is admin-imported reference data, so a
// pattern row must reach the model with no boundary tags at all — not on the
// technique name, not on its platform list.
//
// A patternId is not unique: TheHive 5.6 imports its own ATT&CK catalog in the
// background after startup (as system@thehive.local), next to the one the
// harness imports, so T1059 comes back once or twice depending on how far that
// import has got. Every row is checked rather than assuming a single match.
func TestSearchPatternsCarryNoUntrustedFence(t *testing.T) {
	testutils.Parallel(t)
	testutils.RequireAttackPattern(t)

	testutils.SetupTestWithCleanup(t)

	mcpClient := newSearchClient(t)

	patterns := searchRows(t, mcpClient, map[string]any{
		pEntityType: types.EntityTypePattern,
		pFilters: map[string]any{
			tIn: map[string]any{
				tField:  "patternId",
				tValues: []string{testutils.TestMITREPatternID},
			},
		},
		pExtraColumns: []string{"description", "url", "platforms"},
	})
	require.NotEmpty(t, patterns, "the harness imports %s, so the filter must match it", testutils.TestMITREPatternID)

	for _, row := range patterns {
		pattern, ok := row.(map[string]any)
		require.True(t, ok, "pattern row should be an object, got %T", row)
		require.Equal(t, testutils.TestMITREPatternID, pattern["patternId"])

		requirePatternUnfenced(t, pattern)
	}
}

// requirePatternUnfenced asserts no catalog field of one pattern row carries
// boundary tags.
func requirePatternUnfenced(t *testing.T, pattern map[string]any) {
	t.Helper()

	for _, field := range []string{tName, "description", "url"} {
		value, isString := pattern[field].(string)
		require.True(t, isString, "pattern %v field %q should be a string, got %T", pattern[tID], field, pattern[field])
		require.NotContains(t, value, "UNTRUSTED_DATA",
			"pattern %v catalog field %q must not be fenced: %q", pattern[tID], field, value)
	}

	platforms, ok := pattern["platforms"].([]any)
	require.True(t, ok, "pattern %v should expose a platforms list, got %T", pattern[tID], pattern["platforms"])

	for _, platform := range platforms {
		value, isString := platform.(string)
		require.True(t, isString, "pattern %v platform should be a string, got %T", pattern[tID], platform)
		require.NotContains(t, value, "UNTRUSTED_DATA",
			"pattern %v catalog platform must not be fenced: %q", pattern[tID], value)
	}
}

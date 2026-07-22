package manage

import (
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

func TestNewFilteredOutputPage(t *testing.T) {
	t.Parallel()

	page := &thehive.OutputPage{
		UnderscoreId:        "~123",
		Title:               "Test Page",
		Category:            "Default",
		Order:               1,
		UnderscoreCreatedAt: 1700000000000,
	}

	result := NewFilteredOutputPage(page)

	require.Equal(t, "~123", result.UnderscoreID)
	require.Equal(t, "Test Page", result.Title)
	require.Equal(t, "Default", result.Category)
	require.Equal(t, int32(1), result.Order)
	require.Equal(t, int64(1700000000000), result.CreatedAt)
}

func TestNewCreatePageResult(t *testing.T) {
	t.Parallel()

	page := &thehive.OutputPage{
		UnderscoreId:        "~456",
		Title:               "My Page",
		Category:            "Analysis",
		Order:               0,
		UnderscoreCreatedAt: 1700000000000,
	}

	result := NewCreatePageResult(page)

	require.Equal(t, OperationCreate, result.Operation)
	require.Equal(t, types.EntityTypePage, result.EntityType)
	require.NotNil(t, result.Result)
	require.Equal(t, "~456", result.Result.UnderscoreID)
	require.Equal(t, "My Page", result.Result.Title)
	require.Equal(t, utils.TrustedString("Page created successfully"), result.Message)
}

func TestNewFilteredOutputProcedure_DoesNotPanicWhenTacticIsNil(t *testing.T) {
	t.Parallel()

	patternID := "T1059"

	procedure := &thehive.OutputProcedure{
		UnderscoreId:        "~1",
		UnderscoreCreatedAt: 1700000000000,
		PatternId:           &patternID,
		Tactic:              nil,
	}

	require.NotPanics(t, func() {
		result := NewFilteredOutputProcedure(procedure)
		require.Equal(t, "~1", result.UnderscoreID)
		require.Equal(t, patternID, result.PatternID)
		require.Empty(t, result.Tactic)
	})
}

// TestEnvelopeTrustClassification pins the deliberate trust split on the
// manage envelopes (DL-6703): MCP-authored status text is unwrapped, while
// upstream-derived content keeps its [UNTRUSTED_DATA] tags. If this test fails
// after a type "harmonization", the change reopened an injection channel.
func TestEnvelopeTrustClassification(t *testing.T) {
	t.Parallel()

	merge := NewMergeObservablesResult(`{"deduplicated": ["~1"]}`, "~99")
	update := SingleEntityUpdateResult{EntityID: "~7", Result: resultUpdated}

	processedMerge, err := utils.ProcessDatesRecursive(merge, true)
	require.NoError(t, err)

	mergeMap, ok := processedMerge.(map[string]any)
	require.True(t, ok)
	// API merge summary: upstream content, must stay wrapped.
	require.Equal(t, `[UNTRUSTED_DATA]{"deduplicated": ["~1"]}[/UNTRUSTED_DATA]`, mergeMap["result"])
	require.Equal(t, "Observables merged/deduplicated successfully", mergeMap["message"])

	processedUpdate, err := utils.ProcessDatesRecursive(update, true)
	require.NoError(t, err)

	updateMap, ok := processedUpdate.(map[string]any)
	require.True(t, ok)
	// MCP-authored constant: must not be wrapped.
	require.Equal(t, "updated", updateMap["result"])
}

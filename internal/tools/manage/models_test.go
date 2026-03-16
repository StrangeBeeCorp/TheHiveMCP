package manage

import (
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/require"
)

func TestNewFilteredOutputPage(t *testing.T) {
	page := &thehive.OutputPage{
		UnderscoreId:        "~123",
		Title:               "Test Page",
		Category:            "Default",
		Order:               1,
		UnderscoreCreatedAt: 1700000000000,
	}

	result := NewFilteredOutputPage(page)

	require.Equal(t, "~123", result.UnderscoreId)
	require.Equal(t, "Test Page", result.Title)
	require.Equal(t, "Default", result.Category)
	require.Equal(t, int32(1), result.Order)
	require.Equal(t, int64(1700000000000), result.CreatedAt)
}

func TestNewCreatePageResult(t *testing.T) {
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
	require.Equal(t, "~456", result.Result.UnderscoreId)
	require.Equal(t, "My Page", result.Result.Title)
	require.Equal(t, "Page created successfully", result.Message)
}

func TestNewFilteredOutputProcedure_DoesNotPanicWhenTacticIsNil(t *testing.T) {
	patternID := "T1059"

	procedure := &thehive.OutputProcedure{
		UnderscoreId:        "~1",
		UnderscoreCreatedAt: 1700000000000,
		PatternId:           &patternID,
		Tactic:              nil,
	}

	require.NotPanics(t, func() {
		result := NewFilteredOutputProcedure(procedure)
		require.Equal(t, "~1", result.UnderscoreId)
		require.Equal(t, patternID, result.PatternId)
		require.Empty(t, result.Tactic)
	})
}

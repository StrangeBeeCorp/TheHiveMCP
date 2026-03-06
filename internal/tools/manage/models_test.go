package manage

import (
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/require"
)

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

package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

func testTimeoutErr() error {
	return fmt.Errorf("Post \"https://thehive/api/v1/case\": %w",
		&utils.TheHiveTimeoutError{Method: "POST", Path: "/api/v1/case", Timeout: 2 * time.Minute})
}

func decodeToolError(t *testing.T, s string) map[string]any {
	t.Helper()

	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &decoded))

	return decoded
}

func TestToolError_LeadsWithTheHiveTimeout(t *testing.T) {
	t.Parallel()

	toolErr := NewToolError("failed to update case").Hint("existing hint").Cause(testTimeoutErr())

	decoded := decodeToolError(t, toolErr.Error())
	assert.Equal(t, "failed to update case: TheHive did not answer within 2m0s", decoded["message"])

	hints, ok := decoded["hints"].([]any)
	require.True(t, ok)
	assert.Len(t, hints, 4, "the tool's own hint plus the three timeout hints")
	assert.Equal(t, "existing hint", hints[0])

	asMap := toolErr.ToMap()
	assert.Equal(t, utils.TrustedString("failed to update case: TheHive did not answer within 2m0s"), asMap["message"])
	assert.Len(t, asMap["hints"], 4)

	assert.Len(t, toolErr.hints, 1, "rendering must not grow the stored hints")
}

func TestToolError_OtherCausesUnchanged(t *testing.T) {
	t.Parallel()

	decoded := decodeToolError(t, NewToolError("failed to update case").Cause(errors.New("404")).Error())
	assert.Equal(t, "failed to update case", decoded["message"])
	assert.NotContains(t, decoded, "hints")
}

func TestDescribeHandlerError_BareTimeoutGetsHints(t *testing.T) {
	t.Parallel()

	decoded := decodeToolError(t, describeHandlerError(fmt.Errorf("search failed: %w", testTimeoutErr())))
	assert.Equal(t, "TheHive request failed: TheHive did not answer within 2m0s", decoded["message"])
	assert.Len(t, decoded["hints"], 3)

	assert.Equal(t, "plain", describeHandlerError(errors.New("plain")))
}

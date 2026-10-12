package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A timestamp a model cannot read unambiguously is worse than a raw epoch: the
// previous "02-01-2006T15:04:05" layout was day-first, so 09-10-2026 meant
// either 9 October or 10 September depending on the reader, and it carried no
// offset while rendering in the server's local zone — so the value silently
// shifted with wherever the server happened to run.
func TestTimestampToString_IsRFC3339InUTC(t *testing.T) {
	t.Parallel()

	// 2023-11-14T22:13:20Z
	const epochMillis int64 = 1700000000000

	got := timestampToString(epochMillis)

	require.Equal(t, "2023-11-14T22:13:20Z", got)

	parsed, err := time.Parse(time.RFC3339, got)
	require.NoError(t, err, "every emitted date must parse as RFC 3339")
	require.Equal(t, time.UTC, parsed.Location(), "dates carry an explicit zone, never the server's")
}

func TestTimestampToString_ZeroIsEmpty(t *testing.T) {
	t.Parallel()

	require.Empty(t, timestampToString(0))
}

// A date field holding something other than a number is kept, and logged by its name and type
// only: its value can be entity data, so untrusted.
//
//nolint:paralleltest // mutates the global slog default logger
func TestProcessDateField_LogsNoValue(t *testing.T) {
	buf := captureDebugLogs(t)
	schema := map[string]any{"description": "Date-time string when the incident began", "type": "string"}

	require.Equal(t, schema, processDateField("startDate", schema), "a value that is no timestamp is kept as it is")
	require.Contains(t, buf.String(), "field=startDate")
	require.Contains(t, buf.String(), `type="map[string]interface {}"`)
	require.NotContains(t, buf.String(), "incident began")
}

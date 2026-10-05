package utils

import (
	"fmt"
	"log/slog"
	"slices"
	"time"
)

func timestampToString(ts int64) string {
	if ts == 0 {
		return ""
	}
	// ts is milliseconds (TheHive format).
	//
	// RFC 3339 in UTC, not a local-time layout. The previous format was
	// "02-01-2006T15:04:05": day-first, so 09-10-2026 reads as either 9 October
	// or 10 September depending on the reader, and offset-free while
	// time.UnixMilli renders in the server's zone — so a timestamp came back
	// shifted by the server's offset with nothing to say so.
	t := time.UnixMilli(ts).UTC()

	return t.Format(time.RFC3339)
}

func processDateField(key string, value any) any {
	for _, dateField := range dateFields {
		if key == dateField {
			if value == nil {
				return nil
			}

			var timestamp int64

			switch v := value.(type) {
			case int64:
				timestamp = v
			case float64:
				timestamp = int64(v)
			case int:
				timestamp = int64(v)
			default:
				// The value stays out of the log: it can be entity data, so untrusted.
				slog.Warn("Date field is not a number", "field", key, "type", fmt.Sprintf("%T", value))
				continue
			}

			return timestampToString(timestamp)
		}
	}

	return value
}

func isDateField(fieldName string) bool {
	return slices.Contains(dateFields, fieldName)
}

// IsDateField reports whether a field name carries an epoch timestamp that
// ProcessDatesRecursive rewrites into a formatted string.
//
// Exported so a tool's advertised output schema can be derived from the same
// list that drives the rewrite: the Go field is an integer, the value on the
// wire is a string, and a schema inferred from the struct alone would declare
// the type the client never receives.
func IsDateField(fieldName string) bool { return isDateField(fieldName) }

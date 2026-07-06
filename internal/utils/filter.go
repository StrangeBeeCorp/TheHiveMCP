package utils

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

// CreateFilterFromJSONString parses a JSON object string into a TheHive named
// filter operation.
func CreateFilterFromJSONString(filterString string) (thehive.InputQueryNamedOperation, error) {
	var filterMap map[string]any

	err := json.Unmarshal([]byte(filterString), &filterMap)
	if err != nil {
		return thehive.InputQueryNamedOperation{}, fmt.Errorf("parsing filter JSON: %w", err)
	}

	return thehive.MapmapOfStringAnyAsInputQueryNamedOperation(&filterMap), nil
}

// parseDateStringToTimestamp returns epoch millis.
func parseDateStringToTimestamp(dateStr string) (int64, error) {
	layout := "2006-01-02T15:04:05"

	t, err := time.Parse(layout, dateStr)
	if err != nil {
		return 0, fmt.Errorf("parsing date %q: %w", dateStr, err)
	}

	return t.UnixMilli(), nil
}

// normalizeFilterKey repairs a malformed filter key from a weaker model:
// strips surrounding literal double-quotes (`"_field"` -> `_field`) and
// leading/trailing whitespace. Keys only — never values: a user value like
// {"_value": "*Phishing*"} must reach TheHive byte-for-byte, whereas keys are a
// closed DSL vocabulary, so trimming them is safe.
func normalizeFilterKey(key string) string {
	key = strings.TrimSpace(key)
	// Strip a single matched pair of surrounding double-quotes only — an
	// unbalanced quote is left as-is so it still surfaces TheHive's hinted 400.
	if len(key) >= 2 && strings.HasPrefix(key, `"`) && strings.HasSuffix(key, `"`) {
		key = strings.TrimSpace(key[1 : len(key)-1])
	}

	return key
}

// NormalizeFilterKeys recursively repairs malformed keys in a filter map (see
// normalizeFilterKey), walking nested maps/slices like TranslateDatesToTimestamps.
// Unrecoverable keys fall through so TheHive returns its field-listing 400.
// Mutates and returns filterMap.
//
// Apply only to FILTER maps, never to entity-write payloads — a created entity
// may legitimately carry a field whose name needs no repair.
func NormalizeFilterKeys(filterMap map[string]any) map[string]any {
	// Collect renames first: mutating a map while ranging over it is UB for
	// newly-added keys in Go. Recursion is safe in the same pass (mutates the
	// nested map, not filterMap).
	renames := make(map[string]string)

	for key, value := range filterMap {
		if normalized := normalizeFilterKey(key); normalized != key {
			renames[key] = normalized
		}

		switch v := value.(type) {
		case map[string]any:
			NormalizeFilterKeys(v)
		case []any:
			for i, item := range v {
				if itemMap, ok := item.(map[string]any); ok {
					NormalizeFilterKeys(itemMap)
					v[i] = itemMap
				}
			}
		}
	}

	for old, normalized := range renames {
		// If the clean key already exists (both `_field` and `"_field"` present),
		// keep it and drop the malformed duplicate rather than clobbering.
		if _, clean := filterMap[normalized]; !clean {
			filterMap[normalized] = filterMap[old]
		}

		delete(filterMap, old)
	}

	return filterMap
}

// TranslateDatesToTimestamps recursively converts date strings in a filter map
// to epoch millis.
func TranslateDatesToTimestamps(filterMap map[string]any) map[string]any {
	for key, value := range filterMap {
		switch v := value.(type) {
		case string:
			timestamp, err := parseDateStringToTimestamp(v)
			if err == nil {
				filterMap[key] = timestamp
			}
		case map[string]any:
			TranslateDatesToTimestamps(v)
		case []any:
			for i, item := range v {
				if itemMap, ok := item.(map[string]any); ok {
					TranslateDatesToTimestamps(itemMap)
					v[i] = itemMap
				}
			}
		}
	}

	return filterMap
}

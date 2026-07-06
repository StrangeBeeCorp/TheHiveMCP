package utils

import (
	"encoding/json"
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
		return thehive.InputQueryNamedOperation{}, err
	}

	// Use the generic map converter that's available in the generated client
	return thehive.MapmapOfStringAnyAsInputQueryNamedOperation(&filterMap), nil
}

// parses a string in the format YYYY-MM-DDTHH:mm:SS to a timestamp in milliseconds since epoch
func parseDateStringToTimestamp(dateStr string) (int64, error) {
	layout := "2006-01-02T15:04:05"

	t, err := time.Parse(layout, dateStr)
	if err != nil {
		return 0, err
	}

	return t.UnixMilli(), nil
}

// normalizeFilterKey repairs a structurally-malformed filter key produced by a
// weaker model. Two defects are recovered, in order:
//   - surrounding literal double-quotes, e.g. `"_field"` (the JSON key was
//     itself written with embedded quote characters) -> `_field`
//   - leading/trailing whitespace, e.g. ` _field` -> `_field`
//
// Only the key STRUCTURE is touched; values are never modified. Repairing only
// keys is deliberate: a user value such as {"_value": "*Phishing*"} must reach
// TheHive byte-for-byte, and an entity could legitimately be filtered on a
// quoted/spaced value. Keys, by contrast, are operators and field markers from
// a closed DSL vocabulary, so trimming them is safe.
func normalizeFilterKey(key string) string {
	key = strings.TrimSpace(key)
	// Strip a single matched pair of surrounding double-quotes only — an
	// unbalanced quote is left as-is so it still surfaces TheHive's hinted 400.
	if len(key) >= 2 && strings.HasPrefix(key, `"`) && strings.HasSuffix(key, `"`) {
		key = strings.TrimSpace(key[1 : len(key)-1])
	}

	return key
}

// NormalizeFilterKeys recursively repairs structurally-malformed keys in a
// filter map (see normalizeFilterKey) so that filters from weaker models still
// reach TheHive in valid form. It walks nested maps and slices exactly like
// TranslateDatesToTimestamps. It does NOT validate keys against the DSL: an
// unrecoverable key falls through unchanged so TheHive returns its existing
// field-listing 400. Mutates and returns filterMap.
//
// Apply only to FILTER maps, never to entity-write payloads — a created entity
// may legitimately carry a field whose name needs no repair.
func NormalizeFilterKeys(filterMap map[string]any) map[string]any {
	// Collect renames first: mutating the map (add/delete) while ranging over it
	// has undefined behavior for newly-added keys in Go. Recursion is safe to do
	// in the same pass because it mutates the nested map, not filterMap.
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
		// If the normalized key already exists (e.g. both `_field` and `"_field"`
		// were present), the repaired duplicate would clobber the clean key — keep
		// the clean one and drop the malformed duplicate instead.
		if _, clean := filterMap[normalized]; !clean {
			filterMap[normalized] = filterMap[old]
		}

		delete(filterMap, old)
	}

	return filterMap
}

// TranslateDatesToTimestamps searches the filter map for date strings and
// converts them to timestamps in milliseconds since epoch.
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

package utils

import (
	"encoding/json"
	"time"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

func CreateFilterFromJSONString(filterString string) (thehive.InputQueryNamedOperation, error) {
	var filterMap map[string]interface{}
	if err := json.Unmarshal([]byte(filterString), &filterMap); err != nil {
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

// Searches the filter map for date strings and converts them to timestamps in milliseconds since epoch.
func TranslateDatesToTimestamps(filterMap map[string]interface{}) map[string]interface{} {
	for key, value := range filterMap {
		switch v := value.(type) {
		case string:
			if timestamp, err := parseDateStringToTimestamp(v); err == nil {
				filterMap[key] = timestamp
			}
		case map[string]interface{}:
			TranslateDatesToTimestamps(v)
		case []interface{}:
			for i, item := range v {
				if itemMap, ok := item.(map[string]interface{}); ok {
					TranslateDatesToTimestamps(itemMap)
					v[i] = itemMap
				}
			}
		}
	}
	return filterMap
}

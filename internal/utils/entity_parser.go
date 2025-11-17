package utils

import (
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"
)

func timestampToString(ts int64) string {
	if ts == 0 {
		return ""
	}
	// Assuming ts is in milliseconds (TheHive format)
	t := time.UnixMilli(ts)
	return t.Format("02-01-2006T15:04:05")
}

func GetJSONFields(v interface{}) []string {
	var fields []string

	t := reflect.TypeOf(v)

	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Make sure it's a struct
	if t.Kind() != reflect.Struct {
		return fields
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Get the json tag
		jsonTag := field.Tag.Get("json")

		// Skip if no json tag or explicitly ignored
		if jsonTag == "" || jsonTag == "-" {
			continue
		}

		// Split by comma to remove options like "omitempty"
		parts := strings.Split(jsonTag, ",")
		fieldName := parts[0]

		fields = append(fields, fieldName)
	}

	return fields
}

var dateFields = []string{
	"date",
	"startDate",
	"endDate",
	"sightedAt",
	"dueDate",
	"occurDate",
	"lastSyncDate",
	"_createdAt",
	"_updatedAt",
	"newDate",
	"inProgressDate",
	"closedDate",
	"importedDate",
	"alertDate",
	"alertNewDate",
	"alertInProgressDate",
	"alertImportedDate",
	"createdAt",
	"updatedAt",
	"lastSuccessDate",
	"lastErrorDate",
	"validFrom",
	"expiresAt",
	"includeInTimeline",
}

func ParseDateFields(entity map[string]interface{}) (map[string]interface{}, error) {
	serialized := make(map[string]interface{})
	for key, value := range entity {
		// Check if the field is a date field and the value is a number
		isDateField := false
		for _, dateField := range dateFields {
			if key == dateField && value != nil {
				isDateField = true
				// Handle both int64 and float64 (JSON unmarshaling converts large numbers to float64)
				var timestamp int64
				switch v := value.(type) {
				case int64:
					timestamp = v
				case float64:
					timestamp = int64(v)
				case int:
					timestamp = int64(v)
				default:
					slog.Error("Date field is not a number", "field", key, "value", value, "type", fmt.Sprintf("%T", value))
					return nil, fmt.Errorf("date field %s is not a number, got %T", key, value)
				}
				serialized[key] = timestampToString(timestamp)
				break
			}
		}
		if !isDateField {
			serialized[key] = value
		}
	}
	return serialized, nil
}

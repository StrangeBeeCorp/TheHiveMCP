package utils

import (
	"fmt"
	"log/slog"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// ParseURIParameters extracts query parameters from a parameters string
func ParseURIParameters(uri string) (string, map[string]any, error) {

	parts := strings.SplitN(uri, "?", 2)
	if len(parts) != 2 {
		return uri, nil, nil // No parameters to parse
	}

	params := parts[1]
	uri = parts[0]

	values, err := url.ParseQuery(params)
	if err != nil {
		return "", nil, err
	}

	result := make(map[string]any)
	for k, v := range values {
		result[k] = v[0] // takes first value if multiple
	}
	return uri, result, nil
}

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

// processDateField converts a date field value to string format if it's a recognized date field
func processDateField(key string, value interface{}) (interface{}, error) {
	// Check if this is a date field
	for _, dateField := range dateFields {
		if key == dateField {
			// Handle nil values
			if value == nil {
				return nil, nil
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
				slog.Error("Date field is not a number", "field", key, "value", value, "type", fmt.Sprintf("%T", value))
				return nil, fmt.Errorf("date field %s is not a number, got %T", key, value)
			}
			return timestampToString(timestamp), nil
		}
	}
	// Not a date field, return as-is
	return value, nil
}

func ParseDateFields[T types.OutputEntity](entity T) (map[string]interface{}, error) {
	val := reflect.ValueOf(entity)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	// Handle map[string]interface{} directly
	if val.Kind() == reflect.Map {
		if mapEntity, ok := any(entity).(map[string]interface{}); ok {
			return parseMapDateFields(mapEntity)
		}
	}

	// Handle struct types
	if val.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected struct or map, got %s", val.Kind())
	}

	return parseStructDateFields(val)
}

func parseMapDateFields(mapEntity map[string]interface{}) (map[string]interface{}, error) {
	serialized := make(map[string]interface{})

	for key, value := range mapEntity {
		processedValue, err := processDateField(key, value)
		if err != nil {
			return nil, err
		}
		serialized[key] = processedValue
	}

	return serialized, nil
}

func parseStructDateFields(val reflect.Value) (map[string]interface{}, error) {
	serialized := make(map[string]interface{})
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" { // skip unexported fields
			continue
		}

		// Get the field name (use json tag if present)
		key := field.Name
		if tag := field.Tag.Get("json"); tag != "" && tag != "-" {
			key = strings.Split(tag, ",")[0]
		}

		value := val.Field(i).Interface()

		// Handle pointer types by dereferencing
		fieldVal := val.Field(i)
		if fieldVal.Kind() == reflect.Ptr && !fieldVal.IsNil() {
			value = fieldVal.Elem().Interface()
		} else if fieldVal.Kind() == reflect.Ptr && fieldVal.IsNil() {
			value = nil
		}

		processedValue, err := processDateField(key, value)
		if err != nil {
			return nil, err
		}
		serialized[key] = processedValue
	}

	return serialized, nil
}

// ParseDateFieldsInArray converts any array of structs to maps and applies date parsing
func ParseDateFieldsInArray[T types.OutputEntity](result []T) ([]map[string]interface{}, error) {
	finalResults := make([]map[string]interface{}, 0, len(result))
	for _, item := range result {
		processed, err := ParseDateFields(item)
		if err != nil {
			return nil, fmt.Errorf("failed to parse date fields: %w", err)
		}
		finalResults = append(finalResults, processed)
	}

	return finalResults, nil
}

package utils

import (
	"fmt"
	"log/slog"
	"net/url"
	"reflect"
	"strings"
	"time"
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

// Unwrapper is implemented by union/sum types that wrap a single active variant.
// When processing results, the wrapper is unwrapped so that only the active
// variant is serialized, avoiding unnecessary nesting with nil sibling fields.
type Unwrapper interface {
	Unwrap() any
}

// UnwrapUnion is a reflection-based helper for union structs whose fields are
// all optional pointer variants. It returns the first non-nil pointer field's
// value, or the original value if none is found. Union types opt in by
// implementing Unwrap() with a one-liner:
//
//	func (r T) Unwrap() any { return utils.UnwrapUnion(r) }
func UnwrapUnion(v any) any {
	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return v
		}
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return v
	}
	for i := 0; i < val.NumField(); i++ {
		f := val.Field(i)
		if f.Kind() == reflect.Ptr && !f.IsNil() {
			return f.Interface()
		}
	}
	return v
}

// ProcessDatesRecursive processes any Go value recursively to convert date fields.
// Handles structs, maps, slices, arrays, and nested combinations.
func ProcessDatesRecursive(value interface{}) (interface{}, error) {
	if value == nil {
		return nil, nil
	}

	// Unwrap union types before processing so the output is flat
	if u, ok := value.(Unwrapper); ok {
		return ProcessDatesRecursive(u.Unwrap())
	}

	val := reflect.ValueOf(value)
	return processDatesValue(val)
}

func processDatesValue(val reflect.Value) (interface{}, error) {
	// Handle pointers
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return nil, nil
		}
		return processDatesValue(val.Elem())
	}

	switch val.Kind() {
	case reflect.Struct:
		return processDatesStruct(val)
	case reflect.Map:
		return processDatesMap(val)
	case reflect.Slice, reflect.Array:
		return processDatesSlice(val)
	case reflect.Interface:
		if val.IsNil() {
			return nil, nil
		}
		return processDatesValue(val.Elem())
	default:
		// For primitive types, return as-is
		return val.Interface(), nil
	}
}

func processDatesStruct(val reflect.Value) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" { // Skip unexported fields
			continue
		}

		// Parse json tag for field name and omitempty option
		key := field.Name
		omitempty := false
		if tag := field.Tag.Get("json"); tag != "" {
			if tag == "-" {
				continue
			}
			parts := strings.Split(tag, ",")
			key = parts[0]
			for _, opt := range parts[1:] {
				if opt == "omitempty" {
					omitempty = true
					break
				}
			}
		}

		fieldVal := val.Field(i)

		// Respect omitempty: skip fields with zero values, matching encoding/json behavior
		if omitempty && fieldVal.IsZero() {
			continue
		}

		var processedValue interface{}
		var err error

		// Check if this is a date field and handle appropriately
		if isDateField(key) {
			processedValue, err = processDateField(key, fieldVal.Interface())
			if err != nil {
				return nil, fmt.Errorf("failed to process date field %s: %w", key, err)
			}
		} else {
			// Recursively process nested structures
			processedValue, err = processDatesValue(fieldVal)
			if err != nil {
				slog.Error("Failed to process nested value in struct", "field", key, "error", err)
				continue // Skip this field but continue processing others
			}
		}

		result[key] = processedValue
	}

	return result, nil
}

func processDatesMap(val reflect.Value) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for _, key := range val.MapKeys() {
		keyStr := fmt.Sprintf("%v", key.Interface())
		mapVal := val.MapIndex(key)

		var processedValue interface{}
		var err error

		// Check if this is a date field
		if isDateField(keyStr) {
			processedValue, err = processDateField(keyStr, mapVal.Interface())
			if err != nil {
				return nil, fmt.Errorf("failed to process date field %s: %w", keyStr, err)
			}
		} else {
			// Recursively process nested structures
			processedValue, err = processDatesValue(mapVal)
			if err != nil {
				return nil, fmt.Errorf("failed to process map value for key %s: %w", keyStr, err)
			}
		}

		result[keyStr] = processedValue
	}

	return result, nil
}

func processDatesSlice(val reflect.Value) ([]interface{}, error) {
	length := val.Len()
	result := make([]interface{}, length)

	for i := 0; i < length; i++ {
		elem := val.Index(i)
		processedElem, err := processDatesValue(elem)
		if err != nil {
			return nil, fmt.Errorf("failed to process slice element %d: %w", i, err)
		}
		result[i] = processedElem
	}

	return result, nil
}

// isDateField checks if a field name is a recognized date field
func isDateField(fieldName string) bool {
	for _, dateField := range dateFields {
		if fieldName == dateField {
			return true
		}
	}
	return false
}

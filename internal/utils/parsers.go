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
	if t.Kind() == reflect.Pointer {
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

// trustedFields is the explicit allowlist of field names whose values are NOT
// wrapped with [UNTRUSTED_DATA] boundary tags. The wrapping policy is
// deny-by-default (DL-6006, RandoriSec 5.4, review M5): every string value is
// treated as untrusted and wrapped UNLESS isTrustedField reports it trusted.
// Anything not trusted — including customFields values, attachment names, and
// fields added to the TheHive SDK in the future — is wrapped automatically, so
// the LLM cannot mistake attacker-controlled free text for instructions.
//
// Trusting a field is a security assertion: "this value is a structural
// identifier, an entity reference, a closed status/type/control value, or a
// user login — never free-form human or attacker text." Wrapping such a value
// would be a functional bug: the model carries the boundary tags into its next
// tool call and corrupts it (a wrapped _id becomes an invalid lookup, a wrapped
// status breaks a filter).
//
// This list is exhaustive by construction, NOT heuristic. It was derived from
// TheHive's OpenAPI schema (v5.7.3) by classifying every field returned by the
// agent-facing entities — case, alert, task, observable, comment, log, page,
// procedure, pattern, case-template, custom-field, attachment, organisation,
// user, share, job, action, audit — together with the MCP tool-result envelope.
// Numbers, booleans and dates are handled elsewhere (numbers/bools are never
// wrapped by wrapUntrustedValue; dates via isDateField), so only string fields
// that are provably non-free-text appear here. Every other string field —
// title, description, message, summary, content, name, displayName, data,
// source, sourceRef, tags, category, type, *Label, url, email, report,
// analyzer/responder names, etc. — is deliberately omitted and therefore
// wrapped.
//
// To re-derive after a TheHive upgrade: re-classify the new OpenAPI schema and
// add ONLY fields you can justify as never carrying free text. When unsure,
// leave a field out — deny-by-default keeps it safe.
var trustedFields = map[string]struct{}{
	// System metadata (underscore-prefixed) and the kind discriminator.
	"_id": {}, "_type": {}, "_createdBy": {}, "_updatedBy": {}, "_kind": {},
	// Entity identifiers and references.
	"id": {}, "caseId": {}, "patternId": {}, "organisationId": {},
	"attachmentId": {}, "rootId": {}, "requestId": {}, "objectId": {},
	"analyzerId": {}, "responderId": {}, "cortexId": {}, "cortexJobId": {},
	// User references — logins, not display names (name/displayName are wrapped).
	"login": {}, "assignee": {}, "owner": {}, "createdBy": {}, "updatedBy": {},
	// Closed status/stage/type control values the agent filters and acts on.
	// dataType is an open vocabulary but is a required tool input (creating and
	// filtering observables), so wrapping it would break that flow.
	"status": {}, "stage": {}, "impactStatus": {}, "dataType": {}, "objectType": {},
	// MCP tool-result envelope: server-generated control values and the ids the
	// result reports back. These are our own stable result structs, not TheHive
	// SDK fields, so trusting them by name carries no drift risk.
	"operation": {}, "entityType": {}, "templateId": {}, "caseIds": {},
	"commentId": {}, "entityId": {}, "entityIds": {}, "jobId": {}, "actionId": {},
	"targetId": {},
}

// isTrustedField reports whether a field's value may be returned to the LLM
// without [UNTRUSTED_DATA] wrapping. Date fields are trusted by definition
// (converted to fixed-format timestamps, never free text); every other trusted
// name is enumerated explicitly in trustedFields.
func isTrustedField(fieldName string) bool {
	if fieldName == "" {
		return false
	}
	if isDateField(fieldName) {
		return true
	}
	_, ok := trustedFields[fieldName]
	return ok
}

// wrapUntrustedValue wraps a string or slice of strings with boundary tags.
// Any occurrences of the boundary markers inside the value are escaped first
// to prevent an attacker from prematurely closing/opening the boundary.
func wrapUntrustedValue(value interface{}) interface{} {
	switch v := value.(type) {
	case string:
		escaped := strings.ReplaceAll(v, "[UNTRUSTED_DATA]", "[ESCAPED_UNTRUSTED_DATA]")
		escaped = strings.ReplaceAll(escaped, "[/UNTRUSTED_DATA]", "[/ESCAPED_UNTRUSTED_DATA]")
		return "[UNTRUSTED_DATA]" + escaped + "[/UNTRUSTED_DATA]"
	case []interface{}:
		wrapped := make([]interface{}, len(v))
		for i, item := range v {
			wrapped[i] = wrapUntrustedValue(item)
		}
		return wrapped
	default:
		return value
	}
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
				slog.Warn("Date field is not a number", "field", key, "value", value, "type", fmt.Sprintf("%T", value))
				continue
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
	if val.Kind() == reflect.Pointer {
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
		if f.Kind() == reflect.Pointer && !f.IsNil() {
			return f.Interface()
		}
	}
	return v
}

// ProcessDatesRecursive processes any Go value recursively to convert date fields.
// Handles structs, maps, slices, arrays, and nested combinations.
// When wrapUntrusted is true, user-generated fields are wrapped with
// [UNTRUSTED_DATA]...[/UNTRUSTED_DATA] boundary tags.
func ProcessDatesRecursive(value interface{}, wrapUntrusted bool) (interface{}, error) {
	if value == nil {
		return nil, nil
	}

	// Unwrap union types before processing so the output is flat
	if u, ok := value.(Unwrapper); ok {
		return ProcessDatesRecursive(u.Unwrap(), wrapUntrusted)
	}

	val := reflect.ValueOf(value)
	return processDatesValue(val, wrapUntrusted)
}

func processDatesValue(val reflect.Value, wrapUntrusted bool) (interface{}, error) {
	// Handle pointers
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return nil, nil
		}
		return processDatesValue(val.Elem(), wrapUntrusted)
	}

	switch val.Kind() {
	case reflect.Struct:
		return processDatesStruct(val, wrapUntrusted)
	case reflect.Map:
		return processDatesMap(val, wrapUntrusted)
	case reflect.Slice, reflect.Array:
		return processDatesSlice(val, wrapUntrusted)
	case reflect.Interface:
		if val.IsNil() {
			return nil, nil
		}
		return processDatesValue(val.Elem(), wrapUntrusted)
	default:
		// For primitive types, return as-is
		return val.Interface(), nil
	}
}

func processDatesStruct(val reflect.Value, wrapUntrusted bool) (map[string]interface{}, error) {
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
			if parts[0] != "" {
				key = parts[0]
			}
			// If parts[0] is empty (e.g., json:",omitempty"), keep field.Name
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
			// Handle nil pointers for date fields explicitly
			if fieldVal.Kind() == reflect.Pointer && fieldVal.IsNil() {
				processedValue = nil
			} else {
				// For non-nil pointers, get the underlying value
				var dateValue interface{}
				if fieldVal.Kind() == reflect.Pointer {
					dateValue = fieldVal.Elem().Interface()
				} else {
					dateValue = fieldVal.Interface()
				}
				processedValue, err = processDateField(key, dateValue)
				if err != nil {
					return nil, fmt.Errorf("failed to process date field %s: %w", key, err)
				}
			}
		} else {
			// Recursively process nested structures
			processedValue, err = processDatesValue(fieldVal, wrapUntrusted)
			if err != nil {
				slog.Error("Failed to process nested value in struct", "field", key, "error", err)
				continue // Skip this field but continue processing others
			}
		}

		if wrapUntrusted && !isTrustedField(key) {
			processedValue = wrapUntrustedValue(processedValue)
		}
		result[key] = processedValue
	}

	return result, nil
}

func processDatesMap(val reflect.Value, wrapUntrusted bool) (map[string]interface{}, error) {
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
			processedValue, err = processDatesValue(mapVal, wrapUntrusted)
			if err != nil {
				return nil, fmt.Errorf("failed to process map value for key %s: %w", keyStr, err)
			}
		}

		if wrapUntrusted && !isTrustedField(keyStr) {
			processedValue = wrapUntrustedValue(processedValue)
		}
		result[keyStr] = processedValue
	}

	return result, nil
}

func processDatesSlice(val reflect.Value, wrapUntrusted bool) ([]interface{}, error) {
	length := val.Len()
	result := make([]interface{}, length)

	for i := 0; i < length; i++ {
		elem := val.Index(i)
		processedElem, err := processDatesValue(elem, wrapUntrusted)
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

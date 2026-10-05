package utils

import (
	"fmt"
	"net/url"
	"reflect"
	"slices"
	"strings"
)

// ParseURIParameters extracts query parameters from a parameters string
func ParseURIParameters(uri string) (string, map[string]any, error) {
	parts := strings.SplitN(uri, "?", 2)
	if len(parts) != 2 {
		return uri, nil, nil
	}

	params := parts[1]
	uri = parts[0]

	values, err := url.ParseQuery(params)
	if err != nil {
		return "", nil, fmt.Errorf("parsing URI query parameters: %w", err)
	}

	result := make(map[string]any)
	for k, v := range values {
		result[k] = v[0] // takes first value if multiple
	}

	return uri, result, nil
}

// GetJSONFields returns the json tag names of the exported fields of v (a struct
// or pointer to struct), skipping fields with no json tag or a "-" tag.
func GetJSONFields(v any) []string {
	var fields []string

	t := reflect.TypeOf(v)

	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return fields
	}

	for field := range t.Fields() {
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}

		// Strip options like "omitempty".
		parts := strings.Split(jsonTag, ",")
		fieldName := parts[0]

		fields = append(fields, fieldName)
	}

	return fields
}

// Unwrapper is implemented by union/sum types wrapping a single active variant,
// so only that variant serializes instead of a struct full of nil siblings.
type Unwrapper interface {
	Unwrap() any
}

// UnwrapUnion returns the first non-nil pointer field of a union struct, or v
// itself if none. Union types opt in with a one-liner:
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

	for _, f := range val.Fields() {
		if f.Kind() == reflect.Pointer && !f.IsNil() {
			return f.Interface()
		}
	}

	return v
}

// jsonFieldKey derives the output key and omitempty flag from a struct field's
// json tag. skip is true when the field is tagged json:"-".
func jsonFieldKey(field reflect.StructField) (key string, omitempty, skip bool) {
	key = field.Name

	tag := field.Tag.Get("json")
	if tag == "" {
		return key, false, false
	}

	if tag == "-" {
		return "", false, true
	}

	parts := strings.Split(tag, ",")
	if parts[0] != "" { // empty (json:",omitempty") keeps field.Name
		key = parts[0]
	}

	return key, slices.Contains(parts[1:], "omitempty"), false
}

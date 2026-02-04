package utils

import (
	"net/url"
	"strings"
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

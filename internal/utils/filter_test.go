package utils

import (
	"reflect"
	"testing"
)

func TestNormalizeFilterKeys(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]interface{}
		want map[string]interface{}
	}{
		{
			name: "clean filter is unchanged",
			in: map[string]interface{}{
				"_like": map[string]interface{}{"_field": "title", "_value": "*Phishing*"},
			},
			want: map[string]interface{}{
				"_like": map[string]interface{}{"_field": "title", "_value": "*Phishing*"},
			},
		},
		{
			// Exact shape that gemini-3-flash emitted and TheHive 400'd on:
			// every structural key wrapped in literal double-quotes.
			name: "over-quoted keys are repaired, values untouched",
			in: map[string]interface{}{
				`"_like"`: map[string]interface{}{
					`"_field"`: "title",
					`"_value"`: "*Phishing*",
				},
			},
			want: map[string]interface{}{
				"_like": map[string]interface{}{"_field": "title", "_value": "*Phishing*"},
			},
		},
		{
			name: "whitespace around keys is trimmed",
			in: map[string]interface{}{
				" _eq ": map[string]interface{}{" _field": "status", "_value ": "New"},
			},
			want: map[string]interface{}{
				"_eq": map[string]interface{}{"_field": "status", "_value": "New"},
			},
		},
		{
			name: "quoted then spaced keys are both repaired",
			in: map[string]interface{}{
				` "_gte" `: map[string]interface{}{`"_field"`: "severity", "_value": 3},
			},
			want: map[string]interface{}{
				"_gte": map[string]interface{}{"_field": "severity", "_value": 3},
			},
		},
		{
			name: "nested boolean operators recurse into slices",
			in: map[string]interface{}{
				`"_and"`: []interface{}{
					map[string]interface{}{`"_eq"`: map[string]interface{}{`"_field"`: "severity", "_value": 4}},
					map[string]interface{}{"_eq": map[string]interface{}{"_field": "status", "_value": "New"}},
				},
			},
			want: map[string]interface{}{
				"_and": []interface{}{
					map[string]interface{}{"_eq": map[string]interface{}{"_field": "severity", "_value": 4}},
					map[string]interface{}{"_eq": map[string]interface{}{"_field": "status", "_value": "New"}},
				},
			},
		},
		{
			name: "values that look like keys are never touched",
			in: map[string]interface{}{
				"_eq": map[string]interface{}{"_field": "title", "_value": `"quoted value"`},
			},
			want: map[string]interface{}{
				"_eq": map[string]interface{}{"_field": "title", "_value": `"quoted value"`},
			},
		},
		{
			name: "unbalanced quote is left for TheHive to reject",
			in: map[string]interface{}{
				`"_like`: map[string]interface{}{"_field": "title", "_value": "x"},
			},
			want: map[string]interface{}{
				`"_like`: map[string]interface{}{"_field": "title", "_value": "x"},
			},
		},
		{
			name: "repaired key does not clobber an existing clean key",
			in: map[string]interface{}{
				"_field":   "title",
				`"_field"`: "shouldNotWin",
			},
			want: map[string]interface{}{
				"_field": "title",
			},
		},
		{
			name: "empty filter is unchanged",
			in:   map[string]interface{}{},
			want: map[string]interface{}{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeFilterKeys(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("NormalizeFilterKeys()\n got = %#v\nwant = %#v", got, tc.want)
			}
		})
	}
}

func TestNormalizeFilterKeyIsIdempotent(t *testing.T) {
	in := map[string]interface{}{
		`"_and"`: []interface{}{
			map[string]interface{}{`"_eq"`: map[string]interface{}{`"_field"`: "severity", "_value": 4}},
		},
	}
	once := NormalizeFilterKeys(in)
	twice := NormalizeFilterKeys(NormalizeFilterKeys(once))
	if !reflect.DeepEqual(once, twice) {
		t.Errorf("normalization is not idempotent:\n once = %#v\ntwice = %#v", once, twice)
	}
}

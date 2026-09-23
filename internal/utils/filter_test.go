package utils

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeFilterKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   map[string]any
		want map[string]any
	}{
		{
			name: "clean filter is unchanged",
			in: map[string]any{
				opLike: map[string]any{fieldField: fieldTitle, fieldValue: filterPhishing},
			},
			want: map[string]any{
				opLike: map[string]any{fieldField: fieldTitle, fieldValue: filterPhishing},
			},
		},
		{
			// Exact shape that gemini-3-flash emitted and TheHive 400'd on:
			// every structural key wrapped in literal double-quotes.
			name: "over-quoted keys are repaired, values untouched",
			in: map[string]any{
				`"_like"`: map[string]any{
					quotedFieldName: "title",
					`"_value"`:      filterPhishing,
				},
			},
			want: map[string]any{
				opLike: map[string]any{fieldField: fieldTitle, fieldValue: filterPhishing},
			},
		},
		{
			name: "whitespace around keys is trimmed",
			in: map[string]any{
				" _eq ": map[string]any{" _field": "status", "_value ": "New"},
			},
			want: map[string]any{
				"_eq": map[string]any{fieldField: fieldStatus, fieldValue: valueNew},
			},
		},
		{
			name: "quoted then spaced keys are both repaired",
			in: map[string]any{
				` "_gte" `: map[string]any{quotedFieldName: fieldSeverity, fieldValue: 3},
			},
			want: map[string]any{
				"_gte": map[string]any{fieldField: fieldSeverity, fieldValue: 3},
			},
		},
		{
			name: "nested boolean operators recurse into slices",
			in: map[string]any{
				`"_and"`: []any{
					map[string]any{quotedOpEq: map[string]any{quotedFieldName: fieldSeverity, fieldValue: 4}},
					map[string]any{opEq: map[string]any{fieldField: fieldStatus, fieldValue: valueNew}},
				},
			},
			want: map[string]any{
				"_and": []any{
					map[string]any{opEq: map[string]any{fieldField: fieldSeverity, fieldValue: 4}},
					map[string]any{opEq: map[string]any{fieldField: fieldStatus, fieldValue: valueNew}},
				},
			},
		},
		{
			name: "values that look like keys are never touched",
			in: map[string]any{
				opEq: map[string]any{fieldField: fieldTitle, fieldValue: `"quoted value"`},
			},
			want: map[string]any{
				opEq: map[string]any{fieldField: fieldTitle, fieldValue: `"quoted value"`},
			},
		},
		{
			name: "unbalanced quote is left for TheHive to reject",
			in: map[string]any{
				`"_like`: map[string]any{fieldField: fieldTitle, fieldValue: "x"},
			},
			want: map[string]any{
				`"_like`: map[string]any{fieldField: fieldTitle, fieldValue: "x"},
			},
		},
		{
			name: "repaired key does not clobber an existing clean key",
			in: map[string]any{
				fieldField:      fieldTitle,
				quotedFieldName: "shouldNotWin",
			},
			want: map[string]any{
				fieldField: fieldTitle,
			},
		},
		{
			name: "empty filter is unchanged",
			in:   map[string]any{},
			want: map[string]any{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := NormalizeFilterKeys(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("NormalizeFilterKeys()\n got = %#v\nwant = %#v", got, tc.want)
			}
		})
	}
}

func TestNormalizeFilterKeyIsIdempotent(t *testing.T) {
	t.Parallel()

	in := map[string]any{
		`"_and"`: []any{
			map[string]any{quotedOpEq: map[string]any{quotedFieldName: fieldSeverity, fieldValue: 4}},
		},
	}

	once := NormalizeFilterKeys(in)

	twice := NormalizeFilterKeys(NormalizeFilterKeys(once))
	if !reflect.DeepEqual(once, twice) {
		t.Errorf("normalization is not idempotent:\n once = %#v\ntwice = %#v", once, twice)
	}
}

func TestReferencedFilterFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   map[string]any
		want []string
	}{
		{
			name: "single comparison",
			in:   map[string]any{opEq: map[string]any{fieldField: fieldStatus, fieldValue: valueNew}},
			want: []string{fieldStatus},
		},
		{
			// _contains names its field directly, without _field/_value.
			name: "presence test takes a bare field name",
			in:   map[string]any{"_contains": fieldTags},
			want: []string{fieldTags},
		},
		{
			name: "boolean operators are walked at any depth",
			in: map[string]any{opAnd: []any{
				map[string]any{opGTE: map[string]any{fieldField: fieldSeverity, fieldValue: 3}},
				map[string]any{"_not": map[string]any{
					opLike: map[string]any{fieldField: fieldTitle, fieldValue: filterPhishing},
				}},
			}},
			want: []string{fieldSeverity, fieldTitle},
		},
		{
			// Repaired by NormalizeFilterKeys before the query, so it counts.
			name: "over-quoted _field key still counts",
			in:   map[string]any{quotedOpEq: map[string]any{quotedFieldName: fieldTitle, fieldValue: filterPhishing}},
			want: []string{fieldTitle},
		},
		{
			// A value equal to a field name is data, not a field reference.
			name: "values are never read as field names",
			in:   map[string]any{opEq: map[string]any{fieldField: fieldTitle, fieldValue: fieldStatus}},
			want: []string{fieldTitle},
		},
		{
			name: "match-all references nothing",
			in:   map[string]any{"_any": map[string]any{}},
			want: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.ElementsMatch(t, tc.want, ReferencedFilterFields(tc.in), "filter %v", tc.in)
		})
	}
}

package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type sampleTarget struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestExtractAndUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    sampleTarget
		wantErr bool
	}{
		{
			name:    "clean json",
			content: `{"name":"alpha","count":3}`,
			want:    sampleTarget{Name: "alpha", Count: 3},
		},
		{
			name:    "json in fenced block with language tag",
			content: "```json\n{\"name\":\"beta\",\"count\":7}\n```",
			want:    sampleTarget{Name: "beta", Count: 7},
		},
		{
			name:    "json in fenced block without language tag",
			content: "```\n{\"name\":\"gamma\",\"count\":1}\n```",
			want:    sampleTarget{Name: "gamma", Count: 1},
		},
		{
			name:    "json surrounded by prose",
			content: "Sure, here is the result:\n{\"name\":\"delta\",\"count\":9}\nLet me know if you need more.",
			want:    sampleTarget{Name: "delta", Count: 9},
		},
		{
			name:    "trailing prose after object",
			content: `{"name":"epsilon","count":2} -- hope this helps!`,
			want:    sampleTarget{Name: "epsilon", Count: 2},
		},
		{
			name:    "multiple json objects takes the first",
			content: `{"name":"first","count":1}{"name":"second","count":2}`,
			want:    sampleTarget{Name: "first", Count: 1},
		},
		{
			name:    "closing brace inside a string value",
			content: `{"name":"a } b","count":5}`,
			want:    sampleTarget{Name: "a } b", Count: 5},
		},
		{
			name:    "object brace inside string with trailing prose",
			content: `{"name":"has } brace","count":4} and some trailing text } here`,
			want:    sampleTarget{Name: "has } brace", Count: 4},
		},
		{
			name:    "not json at all",
			content: `I could not produce a response.`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got sampleTarget
			err := extractAndUnmarshalJSON(tt.content, &got)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

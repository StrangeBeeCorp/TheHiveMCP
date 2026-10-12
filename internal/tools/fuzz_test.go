package tools

import (
	"bytes"
	"encoding/json"
	"testing"
)

// FuzzRetypeDateFields: the union schema's date retyping takes any schema document, and a second
// pass changes nothing, so the schema a tool advertises is stable. `go test` runs the seeds; the
// nightly fuzz gate searches for new inputs.
func FuzzRetypeDateFields(f *testing.F) {
	for _, seed := range []string{
		`{"type": "object", "properties": {"_createdAt": {"type": "integer"}, "title": {"type": "string"}}}`,
		`{"anyOf": [{"properties": {"startDate": {"type": ["integer", "null"]}}}, {"items": {"properties": {"endDate": {}}}}]}`,
		`{"$defs": {"x": {"properties": {"_updatedAt": true, "properties": {"properties": {}}}}}}`,
		`[{"properties": null}, 3, "x", {"properties": {"date": {"type": "integer", "format": "int64"}}}]`,
	} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var document any
		if json.Unmarshal(data, &document) != nil {
			return
		}

		retypeDateFields(document)

		once, err := json.Marshal(document)
		if err != nil {
			t.Fatalf("the retyped schema of %s does not encode: %v", data, err)
		}

		var again any

		err = json.Unmarshal(once, &again)
		if err != nil {
			t.Fatalf("decode %s: %v", once, err)
		}

		retypeDateFields(again)

		twice, err := json.Marshal(again)
		if err != nil {
			t.Fatalf("the twice retyped schema of %s does not encode: %v", data, err)
		}

		// Encodings are compared: json sorts map keys, and a decoded []string is a []any.
		if !bytes.Equal(once, twice) {
			t.Fatalf("a second pass changes the schema of %s:\n%s\n%s", data, once, twice)
		}
	})
}

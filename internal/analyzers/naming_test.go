package analyzers

import (
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A client field is named by its json tag, and without one by the generator's convention
// (UnderscoreCreatedBy is _createdBy, Description is description); a getter is named by the field
// it reads. The test data cannot hold an untagged field: gosec type-checks it against the client,
// whose fields all have tags.
func TestJSONNameFallsBackOnTheGeneratorsConvention(t *testing.T) {
	t.Parallel()

	pkg := types.NewPackage(clientPath+"/thehive", "thehive")
	text := types.Typ[types.String]
	model := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Model", nil), types.NewStruct([]*types.Var{
		types.NewField(token.NoPos, pkg, "UnderscoreCreatedBy", text, false),
		types.NewField(token.NoPos, pkg, "Description", text, false),
		types.NewField(token.NoPos, pkg, "Title", text, false),
	}, []string{"", "", `json:"name,omitempty"`}), nil)

	for field, want := range map[string]string{
		"UnderscoreCreatedBy": "_createdBy",
		"Description":         "description",
		"Title":               "name",
		"Underscore":          "underscore",
	} {
		for _, selector := range []string{field, "Get" + field, "Get" + field + "Ok"} {
			assert.Equal(t, want, jsonName(model, selector), selector)
		}
	}

	assert.Empty(t, jsonName(model, "Get"), "a getter that names no field")
}

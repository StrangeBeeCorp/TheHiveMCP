package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// What TheHive answers to describe: 5.6 gives each attribute its cardinality, 5.5 does not, and
// thehive4go's model requires it.
const (
	describe56 = `{"label":"Case","path":"/case","initialQuery":"listCase","attributes":[` +
		`{"name":"title","cardinality":"single","aggregable":false,"indexType":"standard","type":"string"},` +
		`{"name":"description","cardinality":"single","aggregable":false,"indexType":"fulltextOnly","type":"string"}]}`
	describe55 = `{"label":"Alert","path":"/alert","initialQuery":"listAlert","attributes":[` +
		`{"name":"title","aggregable":false,"indexType":"standard","type":"string"},` +
		`{"name":"description","aggregable":false,"indexType":"fulltextOnly","type":"string"}]}`
)

// schemaOf reads an entity's schema through get-resource from a TheHive that answers describe for
// it. An in-process server serves one deployment, as stdio does, so the server keeps one describe
// per entity type: each case below uses its own.
func schemaOf(t *testing.T, entity, describe string) *mcp.CallToolResult {
	t.Helper()

	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/describe/"+entity {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(describe))
	}))
	t.Cleanup(stub.Close)

	return callTool(t, stub.URL, "", "get-resource", map[string]any{"uri": "hive://schema/" + entity})
}

// The per-field filter modes come from TheHive's describe API, which only 5.6 answers in a form
// the SDK reads. On 5.5 the same schema is served without them, rather than not served.
//
// spec: VER-001
func TestA56OnlyAnnotationIsLeftOutOn55AndNothingBreaks(t *testing.T) {
	t.Parallel()

	on56 := schemaOf(t, "case", describe56)
	require.False(t, on56.IsError, resultText(on56))
	assert.Contains(t, resultText(on56), `"filterable"`, "5.6: the schema says which fields TheHive can filter on")

	on55 := schemaOf(t, "alert", describe55)
	require.False(t, on55.IsError, "5.5: the schema is still served: %s", resultText(on55))
	assert.NotContains(t, resultText(on55), `"filterable"`, "5.5: without the annotation")
	assert.Contains(t, resultText(on55), `"title"`, "5.5: the schema itself")
}

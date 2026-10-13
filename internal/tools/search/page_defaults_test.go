package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/permissions"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

const (
	pageCreatedAt = "_createdAt"
	casePageTitle = "Case notes"
	kbPageTitle   = "Tag dictionary"
)

// fakePages answers TheHive's page list queries the way TheHive does: listPage
// returns the case page and the knowledge-base page alike, with nothing on a row
// to tell them apart, listOrganisationPage returns the knowledge base alone, and
// both drop the fields the query excludes.
func fakePages(t *testing.T) *thehive.APIClient {
	t.Helper()

	pages := []map[string]any{
		{fieldID: "~1", "_type": "Page", "_createdBy": "a@b.c", pageCreatedAt: 1700000000000, fieldTitle: casePageTitle, "content": "notes", "category": "Default", "order": 0},
		{fieldID: "~2", "_type": "Page", "_createdBy": "a@b.c", pageCreatedAt: 1700000000001, fieldTitle: kbPageTitle, "content": "phishing, ransomware", "category": "Dictionaries", "order": 0},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query         []map[string]any `json:"query"`
			ExcludeFields []string         `json:"excludeFields"`
		}
		if r.URL.Path != "/api/v1/query" || json.NewDecoder(r.Body).Decode(&body) != nil || len(body.Query) == 0 {
			w.WriteHeader(http.StatusNotFound)

			return
		}

		rows := pages

		switch body.Query[0]["_name"] {
		case "listPage":
		case "listOrganisationPage":
			rows = pages[1:]
		default:
			w.WriteHeader(http.StatusBadRequest)

			return
		}

		out := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			kept := map[string]any{}

			for k, v := range row {
				if !slices.Contains(body.ExcludeFields, k) {
					kept[k] = v
				}
			}

			out = append(out, kept)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(server.Close)

	cfg := thehive.NewConfiguration()
	cfg.Servers = thehive.ServerConfigurations{{URL: server.URL}}

	return thehive.NewAPIClient(cfg)
}

// searchPages runs a search the way the tool's middleware does: validation,
// then the handler.
func searchPages(t *testing.T, entityType string) ([]map[string]any, error) {
	t.Helper()

	perms, err := permissions.LoadDefault()
	require.NoError(t, err)

	ctx := context.WithValue(context.Background(), types.HiveClientCtxKey, fakePages(t))
	ctx = utils.AddPermissionsToContext(ctx, perms)

	tool := &Tool{}
	params := EntitiesParams{EntityType: entityType}

	err = tool.ValidateParams(&params)
	if err != nil {
		return nil, err
	}

	result, err := tool.Handle(ctx, mcp.CallToolRequest{}, params)

	return result.Results, err
}

func titles(rows []map[string]any) []any {
	out := make([]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, row[fieldTitle])
	}

	return out
}

// The entity types are spelled out rather than taken from the constants, and
// the test has a file of its own, so it builds on a tree without the
// knowledge-base type and fails there for the behaviour, not for a missing symbol.
func TestSearchPages_ReturnTheirTextAndTheKnowledgeBaseAlone(t *testing.T) {
	t.Parallel()

	rows, err := searchPages(t, "page")
	require.NoError(t, err)
	assert.ElementsMatch(t, []any{casePageTitle, kbPageTitle}, titles(rows), "page lists case and knowledge-base pages alike")

	// A row that only names a page answers nothing: content is what a page is read for.
	for _, row := range rows {
		assert.Contains(t, row, "content", "page row %v", row[fieldTitle])
		assert.Contains(t, row, "category", "page row %v", row[fieldTitle])
	}

	rows, err = searchPages(t, "knowledge-base")
	require.NoError(t, err)
	assert.Equal(t, []any{kbPageTitle}, titles(rows), "a knowledge-base search leaves the case page out")
	require.Len(t, rows, 1)
	assert.Equal(t, "phishing, ransomware", rows[0]["content"])
}

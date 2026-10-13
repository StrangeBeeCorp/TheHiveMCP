package testutils

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

// purgeOrgTimeout bounds the delete-until-empty loop in purgeOrg. Two effects
// make a single delete sweep insufficient: the server's list is capped at a
// page window (so a heavy test may need several sweeps to drain), and
// Elasticsearch refreshes its index asynchronously (so a listX right after a
// delete can still return just-deleted rows). purgeOrg re-lists and re-deletes
// until a sweep sees nothing, so the next sequential test starts clean.
const purgeOrgTimeout = 30 * time.Second

// purgeOrg deletes every entity the shared free-mode org accumulated during a
// test, re-sweeping until a full pass finds nothing left. It runs only in
// free-license mode (see testEnvFor), where all tests share one org and run
// sequentially, so a clean slate between tests is what keeps the suite's
// absolute-count assertions valid.
//
// It acts as the test's dedicated user, scoped to the org via the X-Organisation
// header — the same credentials/scope the test itself used. MITRE ATT&CK
// patterns are a global catalog shared by all orgs and are never deleted.
func purgeOrg(t *testing.T, cell *testEnv) {
	t.Helper()

	cfg := &Config{
		URL:      hiveURL,
		Username: cell.username,
		Password: cell.password,
		APIKey:   cell.apiKey,
		OrgName:  cell.org,
	}
	client, ctx := createClientAndContext(t, cfg)

	deadline := time.Now().Add(purgeOrgTimeout)

	for {
		remaining := purgeSweep(ctx, t, client, cell.org)
		if remaining == 0 {
			return
		}

		if time.Now().After(deadline) {
			t.Logf("purgeOrg: %q still had %d entities after %s; next test may see leftovers", cell.org, remaining, purgeOrgTimeout)
			return
		}

		time.Sleep(500 * time.Millisecond)
	}
}

// purgeSweep runs one delete pass over the org's entities and returns how many
// it saw (before deleting). A return of 0 means the org listed empty — the
// loop's exit condition. Deletes are best-effort and merely logged on failure:
// a transient error just leaves rows for the next sweep to retry.
func purgeSweep(ctx context.Context, t *testing.T, client *thehive.APIClient, orgName string) int {
	t.Helper()

	seen := 0

	// Delete cases first: DeleteCase cascades a case's tasks, observables, pages,
	// procedures and comments, so afterwards listPage returns only standalone
	// org pages.
	caseIDs := listEntityIDs(ctx, t, client, "listCase")
	seen += len(caseIDs)

	for _, id := range caseIDs {
		resp, err := client.CaseAPI.DeleteCase(ctx, id).Execute()
		closeResponse(resp)

		if err != nil {
			t.Logf("purgeOrg: delete case %s in %q: %v", id, orgName, err)
		}
	}

	if alertIDs := listEntityIDs(ctx, t, client, "listAlert"); len(alertIDs) > 0 {
		seen += len(alertIDs)

		body := thehive.NewDeleteAlertInBulkRequest(alertIDs)
		resp, err := client.AlertAPI.DeleteAlertInBulk(ctx).DeleteAlertInBulkRequest(*body).Execute()
		closeResponse(resp)

		if err != nil {
			t.Logf("purgeOrg: bulk-delete alerts in %q: %v", orgName, err)
		}
	}

	caseTemplateIDs := listEntityIDs(ctx, t, client, "listCaseTemplate")
	seen += len(caseTemplateIDs)

	for _, id := range caseTemplateIDs {
		resp, err := client.CaseTemplateAPI.DeleteCaseTemplate(ctx, id).Execute()
		closeResponse(resp)

		if err != nil {
			t.Logf("purgeOrg: delete case template %s in %q: %v", id, orgName, err)
		}
	}

	pageIDs := listEntityIDs(ctx, t, client, "listPage")
	seen += len(pageIDs)

	for _, id := range pageIDs {
		resp, err := client.PageAPI.DeleteAPage(ctx, id).Execute()
		closeResponse(resp)

		if err != nil {
			t.Logf("purgeOrg: delete page %s in %q: %v", id, orgName, err)
		}
	}

	return seen
}

// purgeListPageSize bounds the page appended to every purge listing. TheHive's
// /api/v1/query returns only a capped default window when no `page` operation is
// present, so a bare listX could miss rows a heavier test created and leave them
// undeleted. We request an explicit wide page (matching what the production
// search handler does — see buildPagingOperation) so a single call sees every
// entity; purgeOrg still loops in case a test somehow exceeds even this.
const purgeListPageSize = 10000

// listEntityIDs runs a generic list operation (e.g. "listCase") in the current
// org context and returns the _id of every result, up to purgeListPageSize. It
// reuses the same QueryAPI generic-op pattern as tryEnsureTestOrganisation, plus
// an explicit wide `page` op so it is not truncated by the server's default page
// window. A failed query yields an empty slice (the caller retries), never a
// fatal error.
func listEntityIDs(ctx context.Context, t *testing.T, client *thehive.APIClient, operation string) []string {
	t.Helper()

	genericOp := thehive.NewInputQueryGenericOperation(operation)
	pageOp := thehive.NewInputQueryPagingOperation(0, purgeListPageSize, "page")
	query := thehive.NewInputQuery()
	query.SetQuery([]thehive.InputQueryNamedOperation{
		thehive.InputQueryGenericOperationAsInputQueryNamedOperation(genericOp),
		thehive.InputQueryPagingOperationAsInputQueryNamedOperation(pageOp),
	})

	resp, httpResp, err := client.QueryAndExportAPI.QueryAPI(ctx).InputQuery(*query).Execute()
	closeResponse(httpResp)

	if err != nil || httpResp == nil || httpResp.StatusCode != http.StatusOK || resp == nil {
		return nil
	}

	return extractIDs(resp)
}

// extractIDs pulls the "_id" of each object from a generic-op query response.
func extractIDs(resp any) []string {
	jsonBytes, err := json.Marshal(resp)
	if err != nil || jsonBytes == nil {
		return nil
	}

	var rows []struct {
		ID string `json:"_id"`
	}
	if json.Unmarshal(jsonBytes, &rows) != nil {
		return nil
	}

	ids := make([]string, 0, len(rows))

	for _, r := range rows {
		if r.ID != "" {
			ids = append(ids, r.ID)
		}
	}

	return ids
}

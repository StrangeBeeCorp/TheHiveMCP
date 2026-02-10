package manage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

func (t *ManageTool) handleMerge(ctx context.Context, params *ManageEntityParams) (ManageEntityResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return ManageEntityResult{}, tools.NewToolError("failed to get TheHive client").Cause(err).
			Hint("Check your authentication and connection settings")
	}

	switch params.EntityType {
	case types.EntityTypeCase:
		return t.mergeCases(ctx, hiveClient, params.EntityIDs)
	case types.EntityTypeAlert:
		return t.mergeAlertsIntoCase(ctx, hiveClient, params.EntityIDs, params.TargetID)
	case types.EntityTypeObservable:
		return t.mergeObservables(ctx, hiveClient, params.TargetID)
	default:
		return ManageEntityResult{}, tools.NewToolErrorf("merge operation not supported for entity type: %s", params.EntityType)
	}
}

func (t *ManageTool) mergeCases(ctx context.Context, client *thehive.APIClient, caseIDs []string) (ManageEntityResult, error) {
	// MergeCases expects comma-separated case IDs as a string
	idsString := ""
	for i, id := range caseIDs {
		if i > 0 {
			idsString += ","
		}
		idsString += id
	}

	result, resp, err := client.CaseAPI.MergeCases(ctx, idsString).Execute()
	if err != nil {
		return ManageEntityResult{}, tools.NewToolErrorf("failed to merge cases %v", caseIDs).Cause(err).
			Hint("Check that all cases exist and you have permissions").API(resp)
	}

	return ManageEntityResult{
		MergeCasesResult: NewMergeCasesResult(result, caseIDs),
	}, nil
}

func (t *ManageTool) mergeAlertsIntoCase(ctx context.Context, client *thehive.APIClient, alertIDs []string, targetCaseID string) (ManageEntityResult, error) {
	// Use bulk merge if multiple alerts, otherwise single merge
	var result *thehive.OutputCase
	var resp *http.Response
	var err error

	if len(alertIDs) == 1 {
		// Single alert merge
		result, resp, err = client.AlertAPI.MergeAlertWithCase(ctx, alertIDs[0], targetCaseID).Execute()
	} else {
		// Bulk merge
		inputMerge := thehive.InputAlertsMergeWithCase{
			AlertIds: alertIDs,
			CaseId:   targetCaseID,
		}
		result, resp, err = client.AlertAPI.MergeBulkAlertsWithCase(ctx).InputAlertsMergeWithCase(inputMerge).Execute()
	}

	if err != nil {
		return ManageEntityResult{}, tools.NewToolErrorf("failed to merge alerts %v into case %s", alertIDs, targetCaseID).Cause(err).
			Hint("Check that alerts and case exist and you have permissions").API(resp)
	}

	return ManageEntityResult{
		MergeAlertsResult: NewMergeAlertsResult(result, alertIDs, targetCaseID),
	}, nil
}

func (t *ManageTool) mergeObservables(ctx context.Context, client *thehive.APIClient, targetCaseID string) (ManageEntityResult, error) {
	result, resp, err := client.CaseAPI.MergeSimilarObservablesOfThisCase(ctx, targetCaseID).Execute()
	if err != nil {
		return ManageEntityResult{}, tools.NewToolErrorf("failed to merge/deduplicate observables in case %s", targetCaseID).Cause(err).
			Hint("Check that the case exists and you have permissions").API(resp)
	}

	// The API returns summary information about the merge operation
	// Convert result to a map to avoid type marshalling issues
	var resultData string
	if result != nil {
		// Parse the result as JSON to ensure proper serialization
		jsonBytes, marshalErr := json.Marshal(result)
		if marshalErr == nil {
			err = json.Unmarshal(jsonBytes, &resultData)
			if err != nil {
				resultData = fmt.Sprintf("merge completed, but failed to parse result: %v", err)
			}
		} else {
			resultData = "merge completed"
		}
	} else {
		resultData = "merge completed"
	}

	return ManageEntityResult{
		MergeObservablesResult: NewMergeObservablesResult(resultData, targetCaseID),
	}, nil
}

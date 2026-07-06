package manage

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/StrangeBeeCorp/thehive4go/thehive"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

func (t *Tool) handleMerge(ctx context.Context, params *EntityParams) (EntityResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return EntityResult{}, tools.NewToolError("failed to get TheHive client").Cause(err).
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
		return EntityResult{}, tools.NewToolErrorf("merge operation not supported for entity type: %s", params.EntityType)
	}
}

func (t *Tool) mergeCases(ctx context.Context, client *thehive.APIClient, caseIDs []string) (EntityResult, error) {
	// MergeCases expects comma-separated case IDs as a string
	idsString := ""

	var idsStringSb36 strings.Builder

	for i, id := range caseIDs {
		if i > 0 {
			idsStringSb36.WriteString(",")
		}

		idsStringSb36.WriteString(id)
	}

	idsString += idsStringSb36.String()

	result, resp, err := client.CaseAPI.MergeCases(ctx, idsString).Execute()
	defer closeResponse(resp)

	if err != nil {
		return EntityResult{}, tools.NewToolErrorf("failed to merge cases %v", caseIDs).Cause(err).
			Hint("Check that all cases exist and you have permissions").API(resp)
	}

	return EntityResult{
		MergeCasesResult: NewMergeCasesResult(result, caseIDs),
	}, nil
}

func (t *Tool) mergeAlertsIntoCase(ctx context.Context, client *thehive.APIClient, alertIDs []string, targetCaseID string) (EntityResult, error) {
	// Use bulk merge if multiple alerts, otherwise single merge
	var (
		result *thehive.OutputCase
		resp   *http.Response
		err    error
	)

	if len(alertIDs) == 1 {
		// Single alert merge
		singleResult, singleResp, singleErr := client.AlertAPI.MergeAlertWithCase(ctx, alertIDs[0], targetCaseID).Execute()
		defer closeResponse(singleResp)

		result, resp, err = singleResult, singleResp, singleErr
	} else {
		// Bulk merge
		inputMerge := thehive.InputAlertsMergeWithCase{
			AlertIds: alertIDs,
			CaseId:   targetCaseID,
		}

		bulkResult, bulkResp, bulkErr := client.AlertAPI.MergeBulkAlertsWithCase(ctx).InputAlertsMergeWithCase(inputMerge).Execute()
		defer closeResponse(bulkResp)

		result, resp, err = bulkResult, bulkResp, bulkErr
	}

	if err != nil {
		return EntityResult{}, tools.NewToolErrorf("failed to merge alerts %v into case %s", alertIDs, targetCaseID).Cause(err).
			Hint("Check that alerts and case exist and you have permissions").API(resp)
	}

	return EntityResult{
		MergeAlertsResult: NewMergeAlertsResult(result, alertIDs, targetCaseID),
	}, nil
}

func (t *Tool) mergeObservables(ctx context.Context, client *thehive.APIClient, targetCaseID string) (EntityResult, error) {
	result, resp, err := client.CaseAPI.MergeSimilarObservablesOfThisCase(ctx, targetCaseID).Execute()
	defer closeResponse(resp)

	if err != nil {
		return EntityResult{}, tools.NewToolErrorf("failed to merge/deduplicate observables in case %s", targetCaseID).Cause(err).
			Hint("Check that the case exists and you have permissions").API(resp)
	}

	// The API returns summary information about the merge operation
	// Convert result to a string representation
	var resultData string

	if result != nil {
		// Convert the result to JSON string for serialization
		jsonBytes, marshalErr := json.Marshal(result)
		if marshalErr == nil {
			resultData = string(jsonBytes)
		} else {
			resultData = "merge completed"
		}
	} else {
		resultData = "merge completed"
	}

	return EntityResult{
		MergeObservablesResult: NewMergeObservablesResult(resultData, targetCaseID),
	}, nil
}

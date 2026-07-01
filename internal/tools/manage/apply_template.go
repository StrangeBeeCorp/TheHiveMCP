package manage

import (
	"context"
	"encoding/json"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

func (t *ManageTool) handleApplyTemplate(ctx context.Context, params *ManageEntityParams) (ManageEntityResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return ManageEntityResult{}, tools.NewToolError("failed to get TheHive client").Cause(err).
			Hint("Check your authentication and connection settings")
	}

	input := thehive.InputApplyCaseTemplateWithIds{
		Ids:          params.EntityIDs,
		CaseTemplate: params.TargetID,
	}

	// UnmarshalJSON requires "ids" and "caseTemplate" present, so merge them in first.
	if params.EntityData != nil {
		merged := make(map[string]interface{}, len(params.EntityData)+2)
		for k, v := range params.EntityData {
			merged[k] = v
		}
		merged["ids"] = params.EntityIDs
		merged["caseTemplate"] = params.TargetID

		jsonData, err := json.Marshal(merged)
		if err != nil {
			return ManageEntityResult{}, tools.NewToolError("failed to marshal apply-template data").Cause(err)
		}
		if err := json.Unmarshal(jsonData, &input); err != nil {
			return ManageEntityResult{}, tools.NewToolError("failed to unmarshal apply-template data").Cause(err).
				Hint("Optional fields: updateTitlePrefix, updateDescription, updateTags, updateSeverity, updateFlag, updateTlp, updatePap, updateCustomFields, importTasks, importPages")
		}
	}

	resp, err := hiveClient.CaseAPI.ApplyCaseTemplateOnExistingCases(ctx).InputApplyCaseTemplateWithIds(input).Execute()
	if err != nil {
		return ManageEntityResult{}, tools.NewToolError("failed to apply case template to cases").Cause(err).
			Hint("Check that the template and case IDs exist and you have permissions").API(resp)
	}

	return ManageEntityResult{
		ApplyTemplateResult: NewApplyTemplateResult(params.TargetID, params.EntityIDs),
	}, nil
}

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

	// Build the apply template input
	input := thehive.InputApplyCaseTemplateWithIds{
		Ids:          params.EntityIDs,
		CaseTemplate: params.TargetID,
	}

	// If entity-data is provided, unmarshal the optional fields.
	// The struct's UnmarshalJSON requires "ids" and "caseTemplate" to be present,
	// so we merge them into the entity-data map before unmarshalling.
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

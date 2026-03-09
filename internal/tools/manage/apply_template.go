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

	// If entity-data is provided, unmarshal the optional fields
	if params.EntityData != nil {
		jsonData, err := json.Marshal(params.EntityData)
		if err != nil {
			return ManageEntityResult{}, tools.NewToolError("failed to marshal apply-template data").Cause(err)
		}
		if err := json.Unmarshal(jsonData, &input); err != nil {
			return ManageEntityResult{}, tools.NewToolError("failed to unmarshal apply-template data").Cause(err).
				Hint("Optional fields: updateTitlePrefix, updateDescription, updateTags, updateSeverity, updateFlag, updateTlp, updatePap, updateCustomFields, importTasks, importPages")
		}
		// Restore required fields that may have been overwritten by the unmarshal
		input.Ids = params.EntityIDs
		input.CaseTemplate = params.TargetID
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

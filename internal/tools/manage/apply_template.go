package manage

import (
	"context"
	"encoding/json"
	"maps"

	"github.com/StrangeBeeCorp/thehive4go/thehive"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

func (t *Tool) handleApplyTemplate(ctx context.Context, params *EntityParams) (EntityResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return EntityResult{}, tools.NewToolError("failed to get TheHive client").Cause(err).
			Hint("Check your authentication and connection settings")
	}

	input := thehive.InputApplyCaseTemplateWithIds{
		Ids:          params.EntityIDs,
		CaseTemplate: params.TargetID,
	}

	// UnmarshalJSON requires "ids" and "caseTemplate" present, so merge them in first.
	if params.EntityData != nil {
		merged := make(map[string]any, len(params.EntityData)+2)
		maps.Copy(merged, params.EntityData)

		merged["ids"] = params.EntityIDs
		merged["caseTemplate"] = params.TargetID

		jsonData, err := json.Marshal(merged)
		if err != nil {
			return EntityResult{}, tools.NewToolError("failed to marshal apply-template data").Cause(err)
		}

		err = json.Unmarshal(jsonData, &input)
		if err != nil {
			return EntityResult{}, tools.NewToolError("failed to unmarshal apply-template data").Cause(err).
				Hint("Optional fields: updateTitlePrefix, updateDescription, updateTags, updateSeverity, updateFlag, updateTlp, updatePap, updateCustomFields, importTasks, importPages")
		}
	}

	resp, err := hiveClient.CaseAPI.ApplyCaseTemplateOnExistingCases(ctx).InputApplyCaseTemplateWithIds(input).Execute()
	defer closeResponse(resp)

	if err != nil {
		return EntityResult{}, tools.NewToolError("failed to apply case template to cases").Cause(err).
			Hint("Check that the template and case IDs exist and you have permissions").API(resp)
	}

	return EntityResult{
		ApplyTemplateResult: NewApplyTemplateResult(params.TargetID, params.EntityIDs),
	}, nil
}

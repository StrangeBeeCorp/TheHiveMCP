package manage

import (
	"context"
	"encoding/json"

	"github.com/StrangeBeeCorp/thehive4go/thehive"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

func (t *Tool) handlePromote(ctx context.Context, params *EntityParams) (EntityResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return EntityResult{}, tools.NewToolError("failed to get TheHive client").Cause(err).
			Hint("Check your authentication and connection settings")
	}

	alertID := params.EntityIDs[0]

	// Create case from alert - use entity-data if provided for case creation parameters
	req := hiveClient.AlertAPI.CreateCaseFromAlert(ctx, alertID)

	// Always provide a JSON body, even if empty
	var inputCase thehive.InputCreateCaseFromAlert

	if params.EntityData != nil {
		jsonData, err := json.Marshal(params.EntityData)
		if err != nil {
			return EntityResult{}, tools.NewToolError("failed to marshal promote data").Cause(err)
		}

		err = json.Unmarshal(jsonData, &inputCase)
		if err != nil {
			return EntityResult{}, tools.NewToolError("failed to unmarshal promote data").Cause(err).
				Hint("Optional fields include 'caseTemplate' for template name")
		}
	}

	req = req.InputCreateCaseFromAlert(inputCase)

	result, resp, err := req.Execute()
	defer closeResponse(resp)

	if err != nil {
		return EntityResult{}, tools.NewToolErrorf("failed to promote alert %s to case", alertID).Cause(err).
			Hint("Check that the alert exists and you have permissions").API(resp)
	}

	return EntityResult{
		PromoteAlertResult: NewPromoteAlertResult(result),
	}, nil
}

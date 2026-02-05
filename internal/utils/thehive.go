package utils

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

// executeQuery builds and executes a query for a parent entity and its children
func executeQuery(ctx context.Context, client *thehive.APIClient, parentOp, parentID, childOp string) ([]map[string]interface{}, error) {
	operation := map[string]interface{}{
		"_name":    parentOp,
		"idOrName": parentID,
	}

	query := thehive.InputQuery{
		Query: []thehive.InputQueryNamedOperation{
			thehive.MapmapOfStringAnyAsInputQueryNamedOperation(&operation),
			thehive.InputQueryGenericOperationAsInputQueryNamedOperation(
				thehive.NewInputQueryGenericOperation(childOp),
			),
		},
	}

	results, resp, err := client.QueryAndExportAPI.QueryAPI(ctx).InputQuery(query).Execute()
	if err != nil {
		return nil, fmt.Errorf("error getting %s for %s ID %s: %w, %v", childOp, parentOp, parentID, err, resp)
	}

	// Convert results to []map[string]interface{}
	resultBytes, err := json.Marshal(results)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal results: %w", err)
	}

	var mapped []map[string]interface{}
	if err := json.Unmarshal(resultBytes, &mapped); err != nil {
		return nil, fmt.Errorf("failed to unmarshal results: %w", err)
	}

	mapped, err = ParseDateFieldsInArray(mapped)
	if err != nil {
		return nil, fmt.Errorf("failed to parse date fields: %w", err)
	}

	return mapped, nil
}

// Case-related functions
func GetTasksFromCaseID(ctx context.Context, client *thehive.APIClient, caseID string) ([]map[string]interface{}, error) {
	return executeQuery(ctx, client, "getCase", caseID, "tasks")
}

func GetObservablesFromCaseID(ctx context.Context, client *thehive.APIClient, caseID string) ([]map[string]interface{}, error) {
	return executeQuery(ctx, client, "getCase", caseID, "observables")
}

func GetCommentsFromCaseID(ctx context.Context, client *thehive.APIClient, caseID string) ([]map[string]interface{}, error) {
	return executeQuery(ctx, client, "getCase", caseID, "comments")
}

func GetPagesFromCaseID(ctx context.Context, client *thehive.APIClient, caseID string) ([]map[string]interface{}, error) {
	return executeQuery(ctx, client, "getCase", caseID, "pages")
}

func GetAttachmentsFromCaseID(ctx context.Context, client *thehive.APIClient, caseID string) ([]map[string]interface{}, error) {
	return executeQuery(ctx, client, "getCase", caseID, "attachments")
}

// Alert-related functions
func GetObservablesFromAlertID(ctx context.Context, client *thehive.APIClient, alertID string) ([]map[string]interface{}, error) {
	return executeQuery(ctx, client, "getAlert", alertID, "observables")
}

func GetCommentsFromAlertID(ctx context.Context, client *thehive.APIClient, alertID string) ([]map[string]interface{}, error) {
	return executeQuery(ctx, client, "getAlert", alertID, "comments")
}

func GetPagesFromAlertID(ctx context.Context, client *thehive.APIClient, alertID string) ([]map[string]interface{}, error) {
	return executeQuery(ctx, client, "getAlert", alertID, "pages")
}

func GetAttachmentsFromAlertID(ctx context.Context, client *thehive.APIClient, alertID string) ([]map[string]interface{}, error) {
	return executeQuery(ctx, client, "getAlert", alertID, "attachments")
}

// Task-related functions
func GetTaskLogsFromTaskID(ctx context.Context, client *thehive.APIClient, taskID string) ([]map[string]interface{}, error) {
	return executeQuery(ctx, client, "getTask", taskID, "logs")
}

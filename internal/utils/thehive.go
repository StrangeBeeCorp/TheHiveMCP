package utils

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

// executeQuery builds and executes a query for a parent entity and its children
func executeQuery(ctx context.Context, client *thehive.APIClient, parentOp, parentID, childOp string) ([]map[string]any, error) {
	operation := map[string]any{
		opNameKey:   parentOp,
		idOrNameKey: parentID,
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
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}

	if err != nil {
		return nil, fmt.Errorf("error getting %s for %s ID %s: %w, %v", childOp, parentOp, parentID, err, resp)
	}

	// Convert results to []map[string]interface{}
	resultBytes, err := json.Marshal(results)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal results: %w", err)
	}

	var mapped []map[string]any

	err = json.Unmarshal(resultBytes, &mapped)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal results: %w", err)
	}

	return mapped, nil
}

// GetTasksFromCaseID returns the tasks of the given case.
func GetTasksFromCaseID(ctx context.Context, client *thehive.APIClient, caseID string) ([]map[string]any, error) {
	return executeQuery(ctx, client, "getCase", caseID, "tasks")
}

// GetObservablesFromCaseID returns the observables of the given case.
func GetObservablesFromCaseID(ctx context.Context, client *thehive.APIClient, caseID string) ([]map[string]any, error) {
	return executeQuery(ctx, client, "getCase", caseID, "observables")
}

// GetCommentsFromCaseID returns the comments of the given case.
func GetCommentsFromCaseID(ctx context.Context, client *thehive.APIClient, caseID string) ([]map[string]any, error) {
	return executeQuery(ctx, client, "getCase", caseID, "comments")
}

// GetPagesFromCaseID returns the pages of the given case.
func GetPagesFromCaseID(ctx context.Context, client *thehive.APIClient, caseID string) ([]map[string]any, error) {
	return executeQuery(ctx, client, "getCase", caseID, "pages")
}

// GetAttachmentsFromCaseID returns the attachments of the given case.
func GetAttachmentsFromCaseID(ctx context.Context, client *thehive.APIClient, caseID string) ([]map[string]any, error) {
	return executeQuery(ctx, client, "getCase", caseID, "attachments")
}

// GetProceduresFromCaseID returns the procedures of the given case.
func GetProceduresFromCaseID(ctx context.Context, client *thehive.APIClient, caseID string) ([]map[string]any, error) {
	return executeQuery(ctx, client, "getCase", caseID, "procedures")
}

// GetSimilarAlertsFromCaseID returns alerts that share observables with the
// given case.
func GetSimilarAlertsFromCaseID(ctx context.Context, client *thehive.APIClient, caseID string) ([]map[string]any, error) {
	return executeQuery(ctx, client, "getCase", caseID, "similarAlertsLight")
}

// GetSimilarCasesFromCaseID is the classic "Similar Cases": other cases that
// share observables with this case.
func GetSimilarCasesFromCaseID(ctx context.Context, client *thehive.APIClient, caseID string) ([]map[string]any, error) {
	return executeQuery(ctx, client, "getCase", caseID, "similarCasesLight")
}

// GetObservablesFromAlertID returns the observables of the given alert.
func GetObservablesFromAlertID(ctx context.Context, client *thehive.APIClient, alertID string) ([]map[string]any, error) {
	return executeQuery(ctx, client, "getAlert", alertID, "observables")
}

// GetCommentsFromAlertID returns the comments of the given alert.
func GetCommentsFromAlertID(ctx context.Context, client *thehive.APIClient, alertID string) ([]map[string]any, error) {
	return executeQuery(ctx, client, "getAlert", alertID, "comments")
}

// GetPagesFromAlertID returns the pages of the given alert.
func GetPagesFromAlertID(ctx context.Context, client *thehive.APIClient, alertID string) ([]map[string]any, error) {
	return executeQuery(ctx, client, "getAlert", alertID, "pages")
}

// GetAttachmentsFromAlertID returns the attachments of the given alert.
func GetAttachmentsFromAlertID(ctx context.Context, client *thehive.APIClient, alertID string) ([]map[string]any, error) {
	return executeQuery(ctx, client, "getAlert", alertID, "attachments")
}

// GetProceduresFromAlertID returns the procedures of the given alert.
func GetProceduresFromAlertID(ctx context.Context, client *thehive.APIClient, alertID string) ([]map[string]any, error) {
	return executeQuery(ctx, client, "getAlert", alertID, "procedures")
}

// GetSimilarCasesFromAlertID returns cases that share observables with the given
// alert.
func GetSimilarCasesFromAlertID(ctx context.Context, client *thehive.APIClient, alertID string) ([]map[string]any, error) {
	return executeQuery(ctx, client, "getAlert", alertID, "similarCasesLight")
}

// GetSimilarAlertsFromAlertID returns other alerts that share observables with
// this alert (alert-to-alert correlation / deduplication).
func GetSimilarAlertsFromAlertID(ctx context.Context, client *thehive.APIClient, alertID string) ([]map[string]any, error) {
	return executeQuery(ctx, client, "getAlert", alertID, "similarAlertsLight")
}

// GetTaskLogsFromTaskID returns the logs of the given task.
func GetTaskLogsFromTaskID(ctx context.Context, client *thehive.APIClient, taskID string) ([]map[string]any, error) {
	return executeQuery(ctx, client, "getTask", taskID, "logs")
}

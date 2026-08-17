// Package resources exposes TheHive metadata, schemas, rules, and facts as MCP
// resources, covering both static embedded content and dynamic API-backed lookups.
package resources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

// SimplifiedUser is a trimmed-down view of a TheHive user for assignment lookups.
type SimplifiedUser struct {
	ID           string `json:"_id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Organisation string `json:"organisation"`
	Profile      string `json:"profile"`
	Type         string `json:"type"`
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

const (
	// cortexRangeAll asks Cortex for the whole result set instead of a window:
	// its range parser maps "all" to (0, Int.MaxValue). The catalog has to be
	// complete before permission filtering, otherwise an allow-list can hide
	// analyzers that merely sat outside the fetched window.
	cortexRangeAll = "all"

	// errListAnalyzers is shared by both catalog fetch paths.
	errListAnalyzers = "failed to find analyzers: %w. Check that Cortex integration is enabled and you have permissions to list analyzers. API response: %v"

	// Query parameter names accepted by the automation catalogs.
	argDataType = "dataType"
	argOffset   = "offset"
	argLimit    = "limit"

	// defaultWorkerPageLimit bounds one catalog page so a large Cortex install
	// does not flood the caller's context. Raise it per call with ?limit=.
	defaultWorkerPageLimit = 50
	maxWorkerPageLimit     = 500
)

// workerPage is a page of Cortex workers (analyzers or responders) together with
// the counters that tell the caller whether it saw the whole catalog. Reporting
// truncation matters: a silently clipped list reads as "this is all there is".
//
// BlockedByPolicy serves the same honesty goal for permissions. The shipped
// read-only default is an empty analyzer allow-list, so an out-of-the-box catalog
// is empty — and an empty catalog is otherwise indistinguishable from "Cortex is
// not connected" or "this deployment has no analyzers".
type workerPage struct {
	Kind            string                 `json:"kind"`
	Total           int                    `json:"total"`
	Returned        int                    `json:"returned"`
	Offset          int                    `json:"offset"`
	Truncated       bool                   `json:"truncated"`
	BlockedByPolicy int                    `json:"blockedByPolicy"`
	Workers         []thehive.OutputWorker `json:"workers"`
}

// newWorkerPage windows allowed, clamping an out-of-range offset to the end of
// the list rather than erroring. blocked is how many workers the permission
// allow-list removed before windowing.
func newWorkerPage(kind string, allowed []thehive.OutputWorker, blocked, offset, limit int) workerPage {
	total := len(allowed)
	offset = min(offset, total)
	end := min(offset+limit, total)

	page := make([]thehive.OutputWorker, 0, end-offset)
	page = append(page, allowed[offset:end]...)

	return workerPage{
		Kind:            kind,
		Total:           total,
		Returned:        len(page),
		Offset:          offset,
		Truncated:       end < total,
		BlockedByPolicy: blocked,
		Workers:         page,
	}
}

// rejectUnknownArguments fails on a query parameter this resource does not
// understand. Ignoring one silently is worse than erroring: '?datatype=ip' (wrong
// case) or '?entityType=observable' on the analyzer catalog — a natural guess,
// since the responder catalog requires exactly those parameters — would otherwise
// return the entire unfiltered catalog and read as a filtered answer.
func rejectUnknownArguments(req mcp.ReadResourceRequest, allowed ...string) error {
	for name := range req.Params.Arguments {
		if !slices.Contains(allowed, name) {
			return fmt.Errorf("unknown query parameter %q. Supported parameters: %s", name, strings.Join(allowed, ", "))
		}
	}

	return nil
}

// stringArgument reads a string query parameter, returning "" when absent.
func stringArgument(req mcp.ReadResourceRequest, name string) string {
	value, _ := req.Params.Arguments[name].(string)

	return value
}

// dataTypeArgument reads the optional dataType filter, rejecting it when present
// and blank. '?dataType=' would otherwise fall back to the unfiltered catalog,
// answering a much broader question than the caller asked.
func dataTypeArgument(req mcp.ReadResourceRequest) (string, error) {
	raw, present := req.Params.Arguments[argDataType]
	if !present {
		return "", nil
	}

	value, isString := raw.(string)
	if !isString {
		return "", fmt.Errorf("%s must be a string, got %T", argDataType, raw)
	}

	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s must not be empty. Omit it entirely to list the whole catalog", argDataType)
	}

	return value, nil
}

// intArgument reads a numeric query parameter. URI parameters arrive as strings,
// but a client calling the resource directly may pass a JSON number.
func intArgument(req mcp.ReadResourceRequest, name string, fallback int) (int, error) {
	raw, present := req.Params.Arguments[name]
	if !present {
		return fallback, nil
	}

	switch value := raw.(type) {
	case string:
		if value == "" {
			return fallback, nil
		}

		parsed, err := strconv.Atoi(value)
		if err != nil {
			return 0, fmt.Errorf("%s must be an integer, got %q", name, value)
		}

		return parsed, nil
	case float64:
		return int(value), nil
	case int:
		return value, nil
	default:
		return 0, fmt.Errorf("%s must be an integer, got %T", name, raw)
	}
}

// paginationArguments resolves the offset/limit window for a worker catalog.
func paginationArguments(req mcp.ReadResourceRequest) (offset, limit int, err error) {
	offset, err = intArgument(req, argOffset, 0)
	if err != nil {
		return 0, 0, err
	}

	limit, err = intArgument(req, argLimit, defaultWorkerPageLimit)
	if err != nil {
		return 0, 0, err
	}

	if offset < 0 {
		return 0, 0, fmt.Errorf("offset must be zero or greater, got %d", offset)
	}

	if limit < 1 {
		return 0, 0, fmt.Errorf("limit must be at least 1, got %d", limit)
	}

	return offset, min(limit, maxWorkerPageLimit), nil
}

func parseUsers(results any) (string, error) {
	// Round-trip through JSON to coerce interface{} into []thehive.OutputUser.
	resultBytes, err := json.Marshal(results)
	if err != nil {
		return "", fmt.Errorf("failed to marshal results: %w", err)
	}

	var users []thehive.OutputUser

	err = json.Unmarshal(resultBytes, &users)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal to OutputUser: %w", err)
	}

	var simplifiedUsers []SimplifiedUser
	for _, user := range users {
		simplifiedUsers = append(simplifiedUsers, SimplifiedUser{
			ID:           user.UnderscoreId,
			Name:         user.Name,
			Email:        derefString(user.Email),
			Organisation: user.Organisation,
			Profile:      user.Profile,
			Type:         string(user.Type),
		})
	}

	simplifiedUsersJSON, err := json.MarshalIndent(simplifiedUsers, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal simplified users: %w", err)
	}

	return string(simplifiedUsersJSON), nil
}

// GetAvailableUsers returns the organisation's users as a JSON resource.
func GetAvailableUsers(ctx context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client from context: %w. Check authentication and connection settings", err)
	}

	operation := thehive.NewInputQueryGenericOperation("listUser")
	hiveQuery := thehive.InputQuery{
		Query: []thehive.InputQueryNamedOperation{
			thehive.InputQueryGenericOperationAsInputQueryNamedOperation(operation),
		},
	}

	results, resp, err := hiveClient.QueryAndExportAPI.QueryAPI(ctx).InputQuery(hiveQuery).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to find users: %w. Check that you have permissions to list users. API response: %v", err, resp)
	}

	usersJSON, err := parseUsers(results)
	if err != nil {
		return nil, fmt.Errorf("failed to parse users: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "hive://metadata/organisation/users",
			MIMEType: mimeApplicationJSON,
			Text:     usersJSON,
		},
	}, nil
}

// GetAvailableCaseTemplates returns the organisation's case templates as a JSON resource.
func GetAvailableCaseTemplates(ctx context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client from context: %w. Check authentication and connection settings", err)
	}

	operation := thehive.NewInputQueryGenericOperation("listCaseTemplate")
	hiveQuery := thehive.InputQuery{
		Query: []thehive.InputQueryNamedOperation{
			thehive.InputQueryGenericOperationAsInputQueryNamedOperation(operation),
		},
	}

	caseTemplates, resp, err := hiveClient.QueryAndExportAPI.QueryAPI(ctx).InputQuery(hiveQuery).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to find case templates: %w. Check that you have permissions to list case templates. API response: %v", err, resp)
	}

	caseTemplatesJSON, err := json.MarshalIndent(caseTemplates, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal case templates: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "hive://metadata/entities/case/templates",
			MIMEType: mimeApplicationJSON,
			Text:     string(caseTemplatesJSON),
		},
	}, nil
}

// listAnalyzers fetches the analyzer catalog from Cortex. With a dataType, only
// the analyzers accepting that observable type are asked for, which Cortex
// resolves itself; without one, the whole catalog is fetched so the caller can
// filter and window it without losing entries to a fetch cap.
//
// Each SDK call stays inside its own branch so this file never names
// *http.Response: importing net/http here makes bodyclose flag every SDK call in
// the package, and those reports are false positives — the generated client
// already reads and closes the body inside Execute().
func listAnalyzers(ctx context.Context, hiveClient *thehive.APIClient, dataType string) ([]thehive.OutputWorker, error) {
	if dataType != "" {
		analyzers, resp, err := hiveClient.CortexAPI.ListAnalyzersByType(ctx, dataType).Execute()
		if err != nil {
			return nil, fmt.Errorf(errListAnalyzers, err, resp)
		}

		return analyzers, nil
	}

	analyzers, resp, err := hiveClient.CortexAPI.ListAnalyzers(ctx).Range_(cortexRangeAll).Execute()
	if err != nil {
		return nil, fmt.Errorf(errListAnalyzers, err, resp)
	}

	return analyzers, nil
}

// GetAvailableAnalyzers returns the Cortex analyzers the session may use as a JSON resource.
//
// Accepts three optional query parameters:
//   - dataType: only analyzers accepting that observable type (hash, ip, domain,
//     …). Cortex resolves it server-side, so this is the cheap way to answer
//     "what can run on this observable?".
//   - offset/limit: window over the allowed analyzers.
//
// The full catalog is fetched (range=all) and permission-filtered BEFORE the
// window is applied. Filtering after a fixed fetch window used to hide every
// allowed analyzer whose position fell outside it.
func GetAvailableAnalyzers(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client from context: %w. Check authentication and connection settings", err)
	}

	err = rejectUnknownArguments(req, argDataType, argOffset, argLimit)
	if err != nil {
		return nil, err
	}

	dataType, err := dataTypeArgument(req)
	if err != nil {
		return nil, err
	}

	analyzers, err := listAnalyzers(ctx, hiveClient, dataType)
	if err != nil {
		return nil, err
	}

	allowed := analyzers

	perms, permErr := utils.GetPermissionsFromContext(ctx)
	if permErr == nil {
		allowed = []thehive.OutputWorker{}

		for _, analyzer := range analyzers {
			if perms.IsAnalyzerAllowed(analyzer.GetId()) || perms.IsAnalyzerAllowed(analyzer.GetName()) {
				allowed = append(allowed, analyzer)
			}
		}
	}

	offset, limit, err := paginationArguments(req)
	if err != nil {
		return nil, err
	}

	page := newWorkerPage("analyzers", allowed, len(analyzers)-len(allowed), offset, limit)

	pageJSON, err := json.MarshalIndent(page, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal analyzers: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "hive://metadata/automation/analyzers",
			MIMEType: mimeApplicationJSON,
			Text:     string(pageJSON),
		},
	}, nil
}

// GetAvailableResponders returns the Cortex responders for the entity named by the
// request's entityType and entityId parameters as a JSON resource.
func GetAvailableResponders(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	err := rejectUnknownArguments(req, "entityType", "entityId", argOffset, argLimit)
	if err != nil {
		return nil, err
	}

	entityType, ok := req.Params.Arguments["entityType"].(string)
	if !ok {
		return nil, errors.New("entityType query parameter is required and must be a string. Example: hive://metadata/automation/responders?entityType=case&entityId=~123456")
	}

	entityID, ok := req.Params.Arguments["entityId"].(string)
	if !ok {
		return nil, errors.New("entityId query parameter is required and must be a string. Example: hive://metadata/automation/responders?entityType=case&entityId=~123456")
	}

	if entityType == "" || entityID == "" {
		return nil, errors.New("entityType and entityId query parameters are required. Example: hive://metadata/automation/responders?entityType=case&entityId=~123456")
	}

	// Both values are interpolated into the Cortex endpoint path; reject
	// anything but a well-formed entity reference before any call.
	err = validateResponderParams(entityType, entityID)
	if err != nil {
		return nil, err
	}

	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client from context: %w. Check authentication and connection settings", err)
	}

	responders, resp, err := hiveClient.CortexAPI.ListResponders(ctx, entityType, entityID).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to find responders for %s %s: %w. Check that Cortex integration is enabled and you have permissions to list responders. API response: %v", entityType, entityID, err, resp)
	}

	allowed := responders

	perms, permErr := utils.GetPermissionsFromContext(ctx)
	if permErr == nil {
		allowed = []thehive.OutputWorker{}

		for _, responder := range responders {
			if perms.IsResponderAllowed(responder.GetId()) || perms.IsResponderAllowed(responder.GetName()) {
				allowed = append(allowed, responder)
			}
		}
	}

	offset, limit, err := paginationArguments(req)
	if err != nil {
		return nil, err
	}

	respondersJSON, err := json.MarshalIndent(newWorkerPage("responders", allowed, len(responders)-len(allowed), offset, limit), "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal responders: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      fmt.Sprintf("hive://metadata/automation/responders?entityType=%s&entityId=%s", entityType, entityID),
			MIMEType: mimeApplicationJSON,
			Text:     string(respondersJSON),
		},
	}, nil
}

// GetAvailableCaseStatuses returns the configured case status values as a JSON resource.
func GetAvailableCaseStatuses(ctx context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client from context: %w. Check authentication and connection settings", err)
	}

	operation := thehive.NewInputQueryGenericOperation("listCaseStatus")
	hiveQuery := thehive.InputQuery{
		Query: []thehive.InputQueryNamedOperation{
			thehive.InputQueryGenericOperationAsInputQueryNamedOperation(operation),
		},
	}

	caseStatuses, resp, err := hiveClient.QueryAndExportAPI.QueryAPI(ctx).InputQuery(hiveQuery).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to find case statuses: %w, %v", err, resp)
	}

	caseStatusesJSON, err := json.MarshalIndent(caseStatuses, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal case statuses: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "hive://metadata/entities/case/statuses",
			MIMEType: mimeApplicationJSON,
			Text:     string(caseStatusesJSON),
		},
	}, nil
}

// GetCurrentUser returns the authenticated user's information as a JSON resource.
func GetCurrentUser(ctx context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client from context: %w. Check authentication and connection settings", err)
	}

	currentUser, resp, err := hiveClient.UserAPI.GetCurrentUserInfo(ctx).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user information: %w. Check authentication status. API response: %v", err, resp)
	}

	currentUserJSON, err := json.MarshalIndent(currentUser, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal current user: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "hive://config/current-user",
			MIMEType: mimeApplicationJSON,
			Text:     string(currentUserJSON),
		},
	}, nil
}

// GetAvailableObservableTypes returns the configured observable data types as a JSON resource.
func GetAvailableObservableTypes(ctx context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client from context: %w. Check authentication and connection settings", err)
	}

	operation := thehive.NewInputQueryGenericOperation("listObservableType")
	hiveQuery := thehive.InputQuery{
		Query: []thehive.InputQueryNamedOperation{
			thehive.InputQueryGenericOperationAsInputQueryNamedOperation(operation),
		},
	}

	observableTypes, resp, err := hiveClient.QueryAndExportAPI.QueryAPI(ctx).InputQuery(hiveQuery).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to find observable types: %w, %v", err, resp)
	}

	observableTypesJSON, err := json.MarshalIndent(observableTypes, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal observable types: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "hive://metadata/entities/observable/types",
			MIMEType: mimeApplicationJSON,
			Text:     string(observableTypesJSON),
		},
	}, nil
}

// GetAvailableCustomFields returns the organisation's custom fields as a JSON resource.
func GetAvailableCustomFields(ctx context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client from context: %w. Check authentication and connection settings", err)
	}

	customFields, resp, err := hiveClient.CustomFieldAPI.ListCustomFields(ctx).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to find custom fields: %w, %v", err, resp)
	}

	customFieldsJSON, err := json.MarshalIndent(customFields, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal custom fields: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "hive://metadata/entities/custom-fields",
			MIMEType: mimeApplicationJSON,
			Text:     string(customFieldsJSON),
		},
	}, nil
}

// GetCurrentPermissions returns the session's active permission configuration as a JSON resource.
func GetCurrentPermissions(ctx context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	permissions, err := utils.GetPermissionsFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get permissions from context: %w", err)
	}

	//nolint:musttag // permissions.Config lives in another package and is tagged for yaml; JSON falls back to field names intentionally here.
	permissionsJSON, err := json.MarshalIndent(permissions, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal permissions: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "hive://config/permissions",
			MIMEType: mimeApplicationJSON,
			Text:     string(permissionsJSON),
		},
	}, nil
}

// RegisterDynamicResources registers the API-backed metadata resources on the registry.
func RegisterDynamicResources(registry *ResourceRegistry) {
	availableUsers := mcp.NewResource(
		"hive://metadata/organisation/users",
		"Users",
		mcp.WithResourceDescription("List of users in the organisation for assignment"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(availableUsers, GetAvailableUsers)

	availableCaseTemplates := mcp.NewResource(
		"hive://metadata/entities/case/templates",
		"Case Templates",
		mcp.WithResourceDescription("Available case templates with predefined tasks and fields"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(availableCaseTemplates, GetAvailableCaseTemplates)

	availableAnalyzers := mcp.NewResource(
		"hive://metadata/automation/analyzers",
		"Analyzers",
		mcp.WithResourceDescription("Available Cortex analyzers for observable enrichment. Optional query parameters: dataType (only analyzers accepting that observable type, e.g. ?dataType=hash), offset and limit (paging)."),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(availableAnalyzers, GetAvailableAnalyzers)

	availableResponders := mcp.NewResource(
		"hive://metadata/automation/responders",
		"Responders",
		mcp.WithResourceDescription("Available Cortex responders for active response. Requires entityType and entityId query parameters; optional offset and limit (paging)."),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(availableResponders, GetAvailableResponders)

	availableCaseStatuses := mcp.NewResource(
		"hive://metadata/entities/case/statuses",
		"Case Statuses",
		mcp.WithResourceDescription("Available status values for cases (New, InProgress, Resolved, etc.)"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(availableCaseStatuses, GetAvailableCaseStatuses)

	currentUser := mcp.NewResource(
		"hive://config/current-user",
		"Current User",
		mcp.WithResourceDescription("Currently authenticated user information"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(currentUser, GetCurrentUser)

	availableObservableTypes := mcp.NewResource(
		"hive://metadata/entities/observable/types",
		"Observable Types",
		mcp.WithResourceDescription("Available observable data types (ip, domain, hash, url, etc.)"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(availableObservableTypes, GetAvailableObservableTypes)

	availableCustomFields := mcp.NewResource(
		"hive://metadata/entities/custom-fields",
		"Custom Fields",
		mcp.WithResourceDescription("Organisation-defined custom fields across all entities"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(availableCustomFields, GetAvailableCustomFields)

	currentPermissions := mcp.NewResource(
		"hive://config/permissions",
		"Current Permissions",
		mcp.WithResourceDescription("Currently active permissions configuration for this session"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(currentPermissions, GetCurrentPermissions)
}

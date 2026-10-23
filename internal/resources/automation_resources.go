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

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

const (
	// cortexRangeAll asks Cortex for the whole result set instead of a window:
	// its range parser maps "all" to (0, Int.MaxValue). The catalog has to be
	// complete before permission filtering, otherwise an allow-list can hide
	// analyzers that merely sat outside the fetched window.
	cortexRangeAll = "all"

	// errListAnalyzers is shared by both catalog fetch paths.
	errListAnalyzers = "failed to find analyzers: %w. Check that Cortex integration is enabled and you have permissions to list analyzers. API response: %s"

	// Query parameter names accepted by the automation catalogs.
	argDataType   = "dataType"
	argOffset     = "offset"
	argLimit      = "limit"
	argEntityType = "entityType"
	argEntityID   = "entityId"

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

	// The value becomes a path segment of the analyzer-by-type endpoint.
	if !tools.IsPathSegment(value) {
		return "", fmt.Errorf("invalid %s %q: an observable type such as ip or domain cannot be a dot segment or contain a slash, a backslash or a control character", argDataType, value)
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

		return parseIntArgument(name, value)
	case float64:
		// Rendered and re-parsed rather than truncated with int(value), so a
		// JSON number is held to the same contract as a numeric string: 1.9,
		// NaN, +Inf and 1e30 are rejected instead of silently becoming 1, or an
		// implementation-defined value for the last three. FormatFloat with
		// precision -1 renders a whole float exactly ("2", never "2.0").
		return parseIntArgument(name, strconv.FormatFloat(value, 'f', -1, 64))
	case int:
		return value, nil
	default:
		return 0, fmt.Errorf("%s must be an integer, got %T", name, raw)
	}
}

// parseIntArgument parses one already-stringified query parameter value.
func parseIntArgument(name, value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer, got %q", name, value)
	}

	return parsed, nil
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
			return nil, fmt.Errorf(errListAnalyzers, err, utils.DescribeHTTPResponse(resp))
		}

		return analyzers, nil
	}

	analyzers, resp, err := hiveClient.CortexAPI.ListAnalyzers(ctx).Range_(cortexRangeAll).Execute()
	if err != nil {
		return nil, fmt.Errorf(errListAnalyzers, err, utils.DescribeHTTPResponse(resp))
	}

	return utils.OutputWorkers(analyzers), nil
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
	// Every argument is resolved before the client is touched, so a caller that
	// passed a bad one is told exactly that rather than getting a connection or
	// authentication error to chase — and never pays for the range=all fetch.
	// GetAvailableResponders validates in the same order.
	err := rejectUnknownArguments(req, argDataType, argOffset, argLimit)
	if err != nil {
		return nil, err
	}

	dataType, err := dataTypeArgument(req)
	if err != nil {
		return nil, err
	}

	offset, limit, err := paginationArguments(req)
	if err != nil {
		return nil, err
	}

	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client from context: %w. Check authentication and connection settings", err)
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
			if perms.IsAnalyzerAllowed(analyzer.GetId(), analyzer.GetName()) {
				allowed = append(allowed, analyzer)
			}
		}
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
	err := rejectUnknownArguments(req, argEntityType, argEntityID, argOffset, argLimit)
	if err != nil {
		return nil, err
	}

	entityType, ok := req.Params.Arguments[argEntityType].(string)
	if !ok {
		return nil, errors.New("entityType query parameter is required and must be a string. Example: hive://metadata/automation/responders?entityType=case&entityId=~123456")
	}

	entityID, ok := req.Params.Arguments[argEntityID].(string)
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

	// Resolved before the fetch: a bad offset/limit is a deterministic input
	// error and should not cost a Cortex round-trip to report.
	offset, limit, err := paginationArguments(req)
	if err != nil {
		return nil, err
	}

	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client from context: %w. Check authentication and connection settings", err)
	}

	listed, resp, err := hiveClient.CortexAPI.ListResponders(ctx, utils.ResponderObjectType(entityType), entityID).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to find responders for %s %s: %w. Check that Cortex integration is enabled and you have permissions to list responders. API response: %s", entityType, entityID, err, utils.DescribeHTTPResponse(resp))
	}

	responders := utils.OutputWorkers(listed)
	allowed := responders

	perms, permErr := utils.GetPermissionsFromContext(ctx)
	if permErr == nil {
		allowed = []thehive.OutputWorker{}

		for _, responder := range responders {
			if perms.IsResponderAllowed(responder.GetId(), responder.GetName()) {
				allowed = append(allowed, responder)
			}
		}
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

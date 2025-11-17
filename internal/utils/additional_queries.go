package utils

import (
	"context"
	"fmt"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

// QueryFunc defines a function that queries data for a single entity
type QueryFunc func(ctx context.Context, client *thehive.APIClient, entityID string) ([]map[string]interface{}, error)

// EntityQueryConfig maps query names to their corresponding query functions
type EntityQueryConfig map[string]QueryFunc

// queryRegistry maps entity types to their available queries
var queryRegistry = map[string]EntityQueryConfig{
	"case": {
		"tasks":       GetTasksFromCaseID,
		"observables": GetObservablesFromCaseID,
		"comments":    GetCommentsFromCaseID,
		"pages":       GetPagesFromCaseID,
		"attachments": GetAttachmentsFromCaseID,
	},
	"alert": {
		"observables": GetObservablesFromAlertID,
		"comments":    GetCommentsFromAlertID,
		"pages":       GetPagesFromAlertID,
		"attachments": GetAttachmentsFromAlertID,
	},
	"task": {
		"task-logs": GetTaskLogsFromTaskID,
	},
	"observable": {
		// No additional queries supported yet
	},
}

// ExpandEntitiesWithQueries expands each entity with its related data inline
func ExpandEntitiesWithQueries(
	ctx context.Context,
	entityType string,
	entities []map[string]interface{},
	additionalQueries []string,
) ([]map[string]interface{}, error) {
	hiveClient, err := GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client: %w", err)
	}

	// Get query config for this entity type
	queryConfig, exists := queryRegistry[entityType]
	if !exists {
		return nil, fmt.Errorf("additional queries not supported for entity type: %s", entityType)
	}

	// Validate all queries before executing
	for _, queryName := range additionalQueries {
		if _, supported := queryConfig[queryName]; !supported {
			return nil, fmt.Errorf("unsupported additional query '%s' for entity type '%s'", queryName, entityType)
		}
	}

	// Expand each entity with its additional data
	for i, entity := range entities {
		entityID, ok := entity["_id"].(string)
		if !ok {
			return nil, fmt.Errorf("entity at index %d missing _id field", i)
		}

		// Execute each requested query for this entity
		for _, queryName := range additionalQueries {
			queryFunc := queryConfig[queryName]

			data, err := queryFunc(ctx, hiveClient, entityID)
			if err != nil {
				return nil, fmt.Errorf("failed to get %s for %s ID %s: %w", queryName, entityType, entityID, err)
			}

			// Add the results directly to the entity
			entities[i][queryName] = data
		}
	}

	return entities, nil
}

// PerformAdditionalQueries collects all additional data separately (your original behavior)
func PerformAdditionalQueries(
	ctx context.Context,
	entityType string,
	entities []map[string]interface{},
	additionalQueries []string,
) (map[string][]interface{}, error) {
	hiveClient, err := GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client: %w", err)
	}

	// Get query config for this entity type
	queryConfig, exists := queryRegistry[entityType]
	if !exists {
		return nil, fmt.Errorf("additional queries not supported for entity type: %s", entityType)
	}

	results := make(map[string][]interface{})

	// Execute each requested query
	for _, queryName := range additionalQueries {
		queryFunc, supported := queryConfig[queryName]
		if !supported {
			return nil, fmt.Errorf("unsupported additional query '%s' for entity type '%s'", queryName, entityType)
		}

		// Execute query for each entity
		for _, entity := range entities {
			entityID, ok := entity["_id"].(string)
			if !ok {
				return nil, fmt.Errorf("entity missing _id field")
			}

			data, err := queryFunc(ctx, hiveClient, entityID)
			if err != nil {
				return nil, fmt.Errorf("failed to get %s for %s ID %s: %w", queryName, entityType, entityID, err)
			}

			// Append all results to flat list
			for _, item := range data {
				results[queryName] = append(results[queryName], item)
			}
		}
	}

	return results, nil
}

// GetSupportedQueries returns the list of supported queries for an entity type
func GetSupportedQueries(entityType string) []string {
	queryConfig, exists := queryRegistry[entityType]
	if !exists {
		return nil
	}

	queries := make([]string, 0, len(queryConfig))
	for queryName := range queryConfig {
		queries = append(queries, queryName)
	}
	return queries
}

// ValidateQuery checks if a query is supported for an entity type
func ValidateQuery(entityType, queryName string) error {
	queryConfig, exists := queryRegistry[entityType]
	if !exists {
		return fmt.Errorf("entity type '%s' not supported", entityType)
	}

	if _, supported := queryConfig[queryName]; !supported {
		return fmt.Errorf("query '%s' not supported for entity type '%s'", queryName, entityType)
	}

	return nil
}

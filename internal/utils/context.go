package utils

import (
	"context"
	"errors"

	"github.com/StrangeBeeCorp/thehive4go/thehive"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/permissions"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// GetHiveClientFromContext returns the TheHive API client stored in the context,
// or an error when none is present.
func GetHiveClientFromContext(ctx context.Context) (*thehive.APIClient, error) {
	client, ok := ctx.Value(types.HiveClientCtxKey).(*thehive.APIClient)
	if !ok || client == nil {
		return nil, errors.New("hive client not found in context")
	}

	return client, nil
}

// GetDefaultCortexIDFromContext retrieves the default Cortex ID from the context
func GetDefaultCortexIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(types.DefaultCortexIDCtxKey).(string); ok {
		return id
	}

	return types.DefaultCortexID
}

// AddPermissionsToContext adds permissions configuration to the context
func AddPermissionsToContext(ctx context.Context, perms *permissions.Config) context.Context {
	return context.WithValue(ctx, types.PermissionsCtxKey, perms)
}

// GetPermissionsFromContext retrieves permissions configuration from the context
func GetPermissionsFromContext(ctx context.Context) (*permissions.Config, error) {
	perms, ok := ctx.Value(types.PermissionsCtxKey).(*permissions.Config)
	if !ok || perms == nil {
		return nil, errors.New("permissions not found in context")
	}

	return perms, nil
}

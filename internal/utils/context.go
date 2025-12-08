package utils

import (
	"context"
	"fmt"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/permissions"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

func GetHiveClientFromContext(ctx context.Context) (*thehive.APIClient, error) {
	client, ok := ctx.Value(types.HiveClientCtxKey).(*thehive.APIClient)
	if !ok || client == nil {
		return nil, fmt.Errorf("hive client not found in context")
	}
	return client, nil
}

// AddPermissionsToContext adds permissions configuration to the context
func AddPermissionsToContext(ctx context.Context, perms *permissions.Config) context.Context {
	return context.WithValue(ctx, types.PermissionsCtxKey, perms)
}

// GetPermissionsFromContext retrieves permissions configuration from the context
func GetPermissionsFromContext(ctx context.Context) (*permissions.Config, error) {
	perms, ok := ctx.Value(types.PermissionsCtxKey).(*permissions.Config)
	if !ok || perms == nil {
		return nil, fmt.Errorf("permissions not found in context")
	}
	return perms, nil
}

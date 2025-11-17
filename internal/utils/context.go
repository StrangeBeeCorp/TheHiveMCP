package utils

import (
	"context"
	"fmt"

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

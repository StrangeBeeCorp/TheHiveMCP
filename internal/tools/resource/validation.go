package resource

import (
	"context"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

// ValidatePermissions verifies the caller is allowed to use the get-resource tool.
func (t *Tool) ValidatePermissions(ctx context.Context, _ GetResourceParams) error {
	permissions, err := utils.GetPermissionsFromContext(ctx)
	if err != nil {
		return tools.NewToolError("failed to get permissions").Cause(err)
	}

	if !permissions.IsToolAllowed(t.Name()) {
		return tools.NewToolErrorf("tool %s is not permitted by your permissions configuration", t.Name())
	}

	return nil
}

// ValidateParams applies defaults and normalizes the requested URI in place.
func (t *Tool) ValidateParams(params *GetResourceParams) error {
	// Apply defaults and normalization
	if params.URI == "" {
		// Default to catalog for browsing
		params.URI = "hive://catalog"
	} else {
		// Normalize URI: add prefix if needed, remove trailing slash
		params.URI = normalizeURI(params.URI)
	}

	return nil
}

// normalizeURI ensures consistent URI format
func normalizeURI(uri string) string {
	// Add hive:// prefix if missing
	if !strings.HasPrefix(uri, "hive://") {
		uri = "hive://" + uri
	}

	// Remove trailing slash for consistency
	uri = strings.TrimSuffix(uri, "/")

	return uri
}

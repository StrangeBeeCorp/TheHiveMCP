package resource

import (
	"context"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

func (t *ResourceTool) ValidatePermissions(ctx context.Context, params GetResourceParams) error {
	permissions, err := utils.GetPermissionsFromContext(ctx)
	if err != nil {
		return tools.NewToolError("failed to get permissions").Cause(err)
	}

	if !permissions.IsToolAllowed(t.Name()) {
		return tools.NewToolErrorf("tool %s is not permitted by your permissions configuration", t.Name())
	}

	return nil
}

func (t *ResourceTool) ValidateParams(params *GetResourceParams) error {
	// Apply defaults and normalization
	if params.URI == "" {
		// Default to catalog for browsing
		params.URI = "hive://catalog"
	} else {
		// Normalize URI: add prefix if needed, remove trailing slash
		params.URI = normalizeURI(params.URI)
	}

	// Validate URI format
	if !strings.HasPrefix(params.URI, "hive://") {
		return tools.NewToolError("URI must start with 'hive://' prefix")
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

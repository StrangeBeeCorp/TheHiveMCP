package manage_test

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/manage"
)

// The permissions reference lists every entity type and operation manage-entities accepts: once
// a profile has entity_permissions, a type or operation it leaves out is denied, so an operator
// who writes a profile from a list missing one denies it without knowing.
func TestPermissionsReferenceListsEveryEntityTypeAndOperation(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile("../../../docs/reference/permissions.md")
	require.NoError(t, err)

	for key, label := range map[string]string{"entity-type": "Entity types", "operation": "Operations"} {
		line := regexp.MustCompile(`(?m)^- ` + label + `: (.*)$`).FindSubmatch(content)
		require.NotNil(t, line, "permissions.md has no %q line", "- "+label+":")

		for _, value := range manage.EntityParamConstraints[key].Enum {
			assert.Contains(t, string(line[1]), "`"+value+"`", "permissions.md %q does not list %s", label, value)
		}
	}
}

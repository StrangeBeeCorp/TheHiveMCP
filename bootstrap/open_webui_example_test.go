package bootstrap_test

import (
	"encoding/json"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/StrangeBeeCorp/TheHiveMCP/bootstrap"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// composeVar is a ${NAME...} reference, resolved by docker compose at run time.
var composeVar = regexp.MustCompile(`\$\{[^}]*\}`)

// toolConnection is the part of an Open WebUI tool-server connection this example depends on.
// Open WebUI v0.11.4 requires url and path (ToolServerConnection, routers/configs.py): without
// path, its admin endpoint fails to list the connections.
type toolConnection struct {
	Type     string  `json:"type"`
	URL      string  `json:"url"`
	Path     *string `json:"path"`
	AuthType string  `json:"auth_type"`
	Key      string  `json:"key"`
}

func composeEnv(t *testing.T, services map[string]struct {
	Environment []string `yaml:"environment"`
}, service string,
) map[string]string {
	t.Helper()

	svc, ok := services[service]
	require.True(t, ok, "the compose file has no %s service", service)

	env := map[string]string{}

	for _, kv := range svc.Environment {
		k, v, _ := strings.Cut(kv, "=")
		env[k] = v
	}

	return env
}

// The Open WebUI example registers TheHiveMCP where the thehivemcp service serves it, with a
// Bearer key the server reads as the TheHive API key, and that service names its TheHive: with
// neither THEHIVE_URL nor THEHIVE_URL_ALLOWLIST, the server refuses every request.
func TestOpenWebUIExampleReachesTheServerItRuns(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../deployment/examples/docker-compose.open-webui.yml")
	require.NoError(t, err)

	var compose struct {
		Services map[string]struct {
			Environment []string `yaml:"environment"`
		} `yaml:"services"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &compose))

	server := composeEnv(t, compose.Services, "thehivemcp")
	ui := composeEnv(t, compose.Services, "open-webui")

	assert.NotEmpty(t, server[string(types.EnvKeyTheHiveURL)], "thehivemcp sets no THEHIVE_URL")
	assert.NotContains(t, server, "ALLOW_ENV_CREDENTIAL_FALLBACK", "the example must keep clients sending their own key")

	var conns []toolConnection
	require.NoError(t, json.Unmarshal([]byte(composeVar.ReplaceAllString(ui["TOOL_SERVER_CONNECTIONS"], "x")), &conns))
	require.Len(t, conns, 1)

	conn := conns[0]
	assert.Equal(t, "mcp", conn.Type)
	assert.NotNil(t, conn.Path, "Open WebUI needs a path on the connection")

	u, err := url.Parse(conn.URL)
	require.NoError(t, err)

	host, port, err := net.SplitHostPort(u.Host)
	require.NoError(t, err)
	assert.Equal(t, "thehivemcp", host)
	assert.Equal(t, server[string(types.EnvKeyMCPPort)], port)

	endpoint := server[string(types.EnvKeyMCPServerEndpoint)]
	if endpoint == "" {
		endpoint = "/mcp" // the server's default, internal/types/options.go
	}

	assert.Equal(t, endpoint, u.Path)

	// Open WebUI sends a bearer connection's key as "Authorization: Bearer <key>".
	assert.Equal(t, "bearer", conn.AuthType)
	assert.Equal(t, "x", bootstrap.ExtractBearerToken("Bearer "+conn.Key), "the server does not read the key Open WebUI sends")
}

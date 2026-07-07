// Command healthcheck is a liveness probe for the TheHiveMCP HTTP server,
// invoked by the container HEALTHCHECK. A static binary rather than a shell
// command because the runtime image is distroless. Reads the same MCP env vars
// as the server so the probe target cannot drift.
//
// The MCP endpoint is a long-lived SSE stream, so the probe reads only the
// status line and closes without draining the body — it must never block.
package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// probeTimeout bounds the whole probe so a hung listener fails the healthcheck
// rather than blocking until the container runtime's own timeout fires.
const probeTimeout = 3 * time.Second

func main() {
	os.Exit(run())
}

func run() int {
	port := os.Getenv(string(types.EnvKeyMCPPort))
	if port == "" {
		fmt.Fprintf(os.Stderr, "healthcheck: %s is not set\n", types.EnvKeyMCPPort)
		return 1
	}

	endpoint := os.Getenv(string(types.EnvKeyMCPServerEndpoint))
	if endpoint == "" {
		endpoint = "/mcp" // mirror the server default in internal/types/options.go
	}

	// Always probe the loopback interface: the probe runs inside the same
	// network namespace as the server, and the bind host may be a wildcard
	// (0.0.0.0) that is not itself a connectable address.
	url := fmt.Sprintf("http://%s%s", net.JoinHostPort("localhost", port), endpoint)

	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	// #nosec G704 -- URL is built from the server's own MCP_PORT/MCP_SERVER_ENDPOINT env vars and always targets loopback; this is the intended healthcheck target.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: build request: %v\n", err)
		return 1
	}

	client := &http.Client{Timeout: probeTimeout}

	// #nosec G704 -- request targets the loopback healthcheck URL derived from the server's own env vars; intended probe, not user-controlled SSRF.
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: request to %s failed: %v\n", url, err)
		return 1
	}
	// Close without draining: the SSE body never ends (see package doc).
	_ = resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fmt.Fprintf(os.Stderr, "healthcheck: unexpected status %d from %s\n", resp.StatusCode, url)
		return 1
	}

	return 0
}

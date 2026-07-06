// Command healthcheck is a tiny, self-contained liveness probe for the
// TheHiveMCP HTTP server, intended to be invoked by a container HEALTHCHECK.
//
// The runtime image is distroless (no shell, no curl/wget), so the probe is a
// standalone static binary rather than a shell command. It reads the same
// MCP_BIND_HOST / MCP_PORT / MCP_SERVER_ENDPOINT environment variables the
// server uses, so the probe target can never drift from where the server
// actually listens.
//
// The MCP endpoint is served as a long-lived Server-Sent Events stream: a GET
// returns the response headers (HTTP 200) and then holds the connection open.
// This probe therefore reads only the status line and closes the connection
// immediately without draining the body — it must never block on the stream.
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
	// Close immediately without reading the body: the endpoint is an SSE stream
	// that never ends, so draining it would block until the timeout.
	_ = resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fmt.Fprintf(os.Stderr, "healthcheck: unexpected status %d from %s\n", resp.StatusCode, url)
		return 1
	}

	return 0
}

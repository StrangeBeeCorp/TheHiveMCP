package bootstrap

import (
	"net/http"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// Bounds on what a client can make the HTTP server hold. mcp-go reads a whole
// request body before the request is authenticated, and its own Start serves
// with a bare http.Server, so a slow or huge request could pin a connection
// and its memory indefinitely.
//
// readTimeout covers the headers and the body only: net/http lifts the read
// deadline once the body has been read, which mcp-go does before handling, so
// a long tool call or an SSE stream outlives it. There is no WriteTimeout, which
// would cut both.
const (
	readHeaderTimeout   = 10 * time.Second
	readTimeout         = 30 * time.Second
	idleTimeout         = 2 * time.Minute
	maxRequestBodyBytes = 4 << 20 // a tool call is JSON well under this; no tool takes file content inline
)

// newHTTPServer builds the http.Server StartHTTPServer listens with. It routes
// the endpoint path to the MCP handler as mcp-go's Start does, which it replaces;
// Start also mounts OAuth protected-resource metadata, which this server does
// not configure.
func newHTTPServer(s *server.MCPServer, options *types.TheHiveMcpDefaultOptions) (*http.Server, *server.StreamableHTTPServer) {
	httpServer := &http.Server{
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
	}

	mcpHandler := server.NewStreamableHTTPServer(s,
		append(StreamableHTTPOptions(options), server.WithStreamableHTTPServer(httpServer))...)

	// Normalized as server.WithEndpointPath does, and as the healthcheck probe does.
	mux := http.NewServeMux()
	mux.Handle("/"+strings.Trim(options.MCPServerEndpointPath, "/"), mcpHandler)
	httpServer.Handler = refuseDeclaredOversize(http.MaxBytesHandler(mux, maxRequestBodyBytes))

	return httpServer, mcpHandler
}

// refuseDeclaredOversize answers 413 to a body declared larger than the cap,
// before any of it is read. MaxBytesHandler still stops a chunked one, which
// declares no length: mcp-go then answers a JSON-RPC parse error.
func refuseDeclaredOversize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > maxRequestBodyBytes {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)

			return
		}

		next.ServeHTTP(w, r)
	})
}

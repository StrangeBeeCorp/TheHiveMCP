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

// newHTTPServer builds the http.Server StartHTTPServer listens with, routing
// the endpoint path to the MCP handler the way mcp-go's Start would.
func newHTTPServer(s *server.MCPServer, options *types.TheHiveMcpDefaultOptions) (*http.Server, *server.StreamableHTTPServer) {
	httpServer := &http.Server{
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
	}

	mcpHandler := server.NewStreamableHTTPServer(s,
		append(StreamableHTTPOptions(options), server.WithStreamableHTTPServer(httpServer))...)

	// Normalized as server.WithEndpointPath does, so both agree on the path.
	mux := http.NewServeMux()
	mux.Handle("/"+strings.Trim(options.MCPServerEndpointPath, "/"), mcpHandler)
	httpServer.Handler = http.MaxBytesHandler(mux, maxRequestBodyBytes)

	return httpServer, mcpHandler
}

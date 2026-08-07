package bootstrap

import (
	"log/slog"

	"github.com/mark3labs/mcp-go/server"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/auth"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/logging"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/resources"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/execute_automation"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/manage"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/resource"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/search"
	"github.com/StrangeBeeCorp/TheHiveMCP/version"
)

// GetMCPServer builds an MCP server configured with TheHiveMCP's capabilities,
// logging hooks, elicitation, and authentication middleware, but without any
// tools registered.
//
// Neither prompts nor resource subscriptions are declared, because neither is
// implemented: nothing calls AddPrompt, and mcp-go has no resources/subscribe
// handler and we have no change events to feed one. Advertising either would
// invite a request that fails. AddPrompt registers its own capability, so adding
// prompts later needs no change here. capabilities_test.go asserts both.
func GetMCPServer() *server.MCPServer {
	mcpServer := server.NewMCPServer(
		"TheHiveMCP",
		version.GetVersion(),
		server.WithToolCapabilities(true),
		// listChanged=true is trivially satisfied: the resource list is fixed at
		// startup, so no notification is ever owed.
		server.WithResourceCapabilities(false, true),
		server.WithHooks(logging.GetLoggingHooks()),
		server.WithElicitation(),
		server.WithToolHandlerMiddleware(auth.AuthenticationMiddleware()),
		server.WithResourceHandlerMiddleware(auth.ResourceAuthenticationMiddleware()),
	)

	return mcpServer
}

// RegisterToolsToMCPServer registers TheHiveMCP's resources and tools (search,
// manage, resource, and execute-automation) on the given MCP server.
func RegisterToolsToMCPServer(mcpServer *server.MCPServer) {
	resourceRegistry := resources.NewResourceRegistry()
	catalogData := resources.GetCatalogData()

	categories, ok := catalogData["categories"].([]map[string]any)
	if !ok {
		slog.Error("Resource catalog is missing category metadata; skipping category registration")
	} else {
		resourceRegistry.RegisterCategoryMetadata(categories)
	}

	resources.RegisterDynamicResources(resourceRegistry)
	resources.RegisterStaticResources(resourceRegistry)
	resourceRegistry.RegisterAll(mcpServer)

	toolRegistry := tools.NewRegistry()
	toolRegistry.Register(search.NewSearchTool())
	toolRegistry.Register(manage.NewManageTool())
	toolRegistry.Register(resource.NewResourceTool(resourceRegistry))
	toolRegistry.Register(execute_automation.NewExecuteAutomationTool())
	toolRegistry.RegisterAll(mcpServer)
}

// GetMCPServerAndRegisterTools builds an MCP server and registers all of
// TheHiveMCP's tools and resources on it, returning the ready-to-serve server.
func GetMCPServerAndRegisterTools() *server.MCPServer {
	mcpServer := GetMCPServer()
	RegisterToolsToMCPServer(mcpServer)

	return mcpServer
}

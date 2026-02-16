package bootstrap

import (
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/auth"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/logging"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/resources"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/execute_automation"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/manage"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/resource"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/search"
	"github.com/StrangeBeeCorp/TheHiveMCP/version"
	"github.com/mark3labs/mcp-go/server"
)

func GetMCPServer() *server.MCPServer {
	mcpServer := server.NewMCPServer(
		"TheHiveMCP",
		version.GetVersion(),
		server.WithToolCapabilities(true),
		server.WithPromptCapabilities(true),
		server.WithResourceCapabilities(true, true),
		server.WithHooks(logging.GetLoggingHooks()),
		server.WithElicitation(),
		server.WithToolHandlerMiddleware(auth.AuthenticationMiddleware()),
		server.WithResourceHandlerMiddleware(auth.ResourceAuthenticationMiddleware()),
	)
	mcpServer.EnableSampling()
	return mcpServer
}

func RegisterToolsToMCPServer(mcpServer *server.MCPServer) {
	resourceRegistry := resources.NewResourceRegistry()
	catalogData := resources.GetCatalogData()
	resourceRegistry.RegisterCategoryMetadata(catalogData["categories"].([]map[string]interface{}))
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

func GetMCPServerAndRegisterTools() *server.MCPServer {
	mcpServer := GetMCPServer()
	RegisterToolsToMCPServer(mcpServer)
	return mcpServer
}

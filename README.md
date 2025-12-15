# TheHiveMCP

<div align="center">
  <img src="docs/images/theHivelogo.png" alt="TheHive Logo" width="600"/>
</div>

[![Go Version](https://img.shields.io/badge/go-1.24.11+-blue.svg)](https://golang.org/doc/go1.24)
[![License](https://img.shields.io/badge/license-AGPL--3.0-blue.svg)](LICENSE)

**Model Context Protocol server for TheHive security platform**

## Overview

TheHiveMCP is an MCP (Model Context Protocol) server that enables AI agents to interact with [TheHive](https://strangebee.com/thehive/) security platform through natural language. Built in Go, it provides a structured interface for security operations, case management, and threat intelligence workflows.

![Demo TheHiveMCP](docs/images/demo-thehivemcp.gif)

### Key features

- **MCP 1.0 compliant** - Full implementation of MCP specification
- **Multiple transport modes**:
  - 🌐 HTTP - Scalable HTTP transport with SSE support
  - 🪠 Stdio - CLI/pipe operations for local integration
- **Comprehensive security operations**:
  - Natural language entity search (alerts, cases, tasks, observables)
  - Full CRUD operations on TheHive entities
  - Cortex analyzer and responder execution
  - Dynamic resource catalog with live metadata

## How It Works

**TheHiveMCP is a connector that enables AI assistants to interact with TheHive security platform.**

This project acts as a **translation layer** between AI assistants (like ChatGPT, Claude, or other LLMs) and TheHive API. It doesn't contain AI itself - instead, it provides AI assistants with the tools they need to understand and work with security data.

When you connect an AI assistant to TheHiveMCP, the AI can:
- Understand TheHive data structure and capabilities
- Translate natural language requests into proper TheHive operations
- Search for security incidents, cases, and threats
- Create and manage investigations
- Execute automated analysis and response actions

**Real-world example:** An analyst using ChatGPT with TheHiveMCP can say *"Show me high-severity phishing alerts from last week"* and ChatGPT will use TheHiveMCP to query TheHive database and present the results in an organized, actionable format.

This enables security teams to **leverage existing AI assistants** for security operations without replacing their current tools or workflows. TheHiveMCP handles the technical complexity of integrating with TheHive, so AI assistants can focus on understanding security context and providing intelligent insights.

## Project Structure

```
TheHiveMCP/
├── cmd/server/         # Main entrypoint
├── bootstrap/          # Server initialization (public API)
├── internal/           # Core components (tools, resources, prompts, utils)
├── deployment/         # Docker configuration
└── Makefile
```

## Get Started

This guide helps you connect TheHiveMCP to popular AI assistants through MCP hosts. Choose your preferred AI assistant below for step-by-step setup instructions.

### What you'll need

- A running **TheHive 5.x** instance with API access
- Your **TheHive API key** and URL
- An AI assistant that supports MCP (Claude Desktop or other MCP clients)

---

### 🖥️ Claude Desktop (recommended)

**Claude Desktop** supports MCPB (Model Context Protocol Binary) files for easy one-click installation of MCP servers like TheHiveMCP.

#### Step 1: Install Claude Desktop
Download and install [Claude Desktop](https://claude.ai/download) for your operating system.

#### Step 2: Download TheHiveMCP MCPB package
Download the appropriate MCPB file for your system from the [latest release](https://github.com/StrangeBeeCorp/TheHiveMCP/releases):

- **macOS (Intel)**: `thehivemcp-v0.2.0-darwin-amd64.mcpb`
- **macOS (Apple Silicon)**: `thehivemcp-v0.2.0-darwin-arm64.mcpb`
- **Windows (64-bit)**: `thehivemcp-v0.2.0-windows-amd64.mcpb`
- **Linux (64-bit)**: `thehivemcp-v0.2.0-linux-amd64.mcpb`
- **Linux (ARM64)**: `thehivemcp-v0.2.0-linux-arm64.mcpb`

#### Step 3: Install the MCPB package
Double-click the downloaded `.mcpb` file. Claude Desktop automatically:
- Installs TheHiveMCP server
- Prompts you to configure your TheHive connection settings
- Adds TheHiveMCP to your available tools

#### Step 4: Configure TheHive connection
When prompted during installation, provide:
- **TheHive URL**: Your TheHive instance URL (for example, `https://thehive.company.com`)
- **API Key**: Your TheHive API key for authentication
- **Organization**: Your TheHive organization name
- **Permissions config**: (Optional) Path to permissions YAML file, defaults to read-only
- **OpenAI API key**: (Optional) For enhanced natural language processing

#### Step 5: Test your setup
After installation, restart Claude Desktop and look for the 🔧 tools icon. Try asking: *"Show me recent high-severity alerts from TheHive"* or *"What security cases are currently open?"*

---

### 🐳 Docker (alternative setup)

If you prefer Docker or need more control over the server configuration:

#### Step 1: Run TheHiveMCP server
```bash
docker run -d \
  --name thehive-mcp \
  -p 8082:8082 \
  -e THEHIVE_URL=https://your-thehive-instance.com \
  -e THEHIVE_API_KEY=your-api-key-here \
  -e MCP_BIND_HOST=0.0.0.0 \
  -e MCP_PORT=8082 \
  strangebee/thehive-mcp:latest

# With custom permissions:
docker run -d \
  --name thehive-mcp \
  -p 8082:8082 \
  -v /host/path/analyst.yaml:/app/permissions.yaml \
  -e PERMISSIONS_CONFIG=/app/permissions.yaml \
  -e THEHIVE_URL=https://your-thehive-instance.com \
  -e THEHIVE_API_KEY=your-api-key-here \
  strangebee/thehive-mcp:latest
```

#### Step 2: Configure your MCP client
Point your MCP client to connect to the HTTP server at `http://localhost:8082/mcp`.

---

### 🔧 Other MCP clients

TheHiveMCP works with any MCP-compatible client. Popular options include:

- **[MCP CLI](https://github.com/modelcontextprotocol/mcp-cli)** - Command-line interface
- **Custom applications** - Using MCP client libraries

For these clients, use either:
- **Binary**: Download the appropriate binary for your platform from [releases](https://github.com/StrangeBeeCorp/TheHiveMCP/releases) and run with `--transport stdio`
- **HTTP server**: Point to `http://localhost:8082/mcp` after running the Docker container above

## Configuration

TheHiveMCP supports three configuration methods with the following priority (highest to lowest):
1. **HTTP request headers** (for HTTP transport only)
2. **Command-line flags**
3. **Environment variables**

<details>
<summary><strong>⚙️ Configuration parameters</strong></summary>

### Configuration parameters

| Parameter | Environment variable | Command-line flag | HTTP header | Default | Description |
|-----------|---------------------|-------------------|-------------|---------|-------------|
| **TheHive connection** |
| TheHive URL | `THEHIVE_URL` | `--thehive-url` | `X-TheHive-Url` | - | TheHive instance URL (required) |
| API key | `THEHIVE_API_KEY` | `--thehive-api-key` | `Authorization` or `X-TheHive-Api-Key` | - | TheHive API key |
| Username | `THEHIVE_USERNAME` | `--thehive-username` | - | - | Username for basic auth |
| Password | `THEHIVE_PASSWORD` | `--thehive-password` | - | - | Password for basic auth |
| Organization | `THEHIVE_ORGANISATION` | `--thehive-organisation` | `X-TheHive-Org` | - | TheHive organization |
| **Permissions** |
| Permissions config | `PERMISSIONS_CONFIG` | `--permissions-config` | - | (read-only) | Path to permissions YAML file |
| **MCP server** |
| Transport type | - | `--transport` | - | `http` | Transport mode: `http` or `stdio` |
| Bind address | `MCP_BIND_HOST` + `MCP_PORT` | `--addr` | - | - | HTTP server bind address (for example, `0.0.0.0:8082`) |
| Endpoint path | `MCP_ENDPOINT_PATH` | `--mcp-endpoint-path` | - | `/mcp` | HTTP endpoint path |
| Heartbeat interval | `MCP_HEARTBEAT_INTERVAL` | `--mcp-heartbeat-interval` | - | `30s` | Heartbeat interval for HTTP connections |
| **OpenAI integration** |
| API key | `OPENAI_API_KEY` | `--openai-api-key` | - | - | OpenAI-compatible API key |
| Base URL | `OPENAI_BASE_URL` | `--openai-base-url` | - | `https://api.openai.com/v1` | OpenAI-compatible API base URL |
| Model | `OPENAI_MODEL` | `--openai-model` | - | `gpt-5` | Model name |
| Max tokens | `OPENAI_MAX_TOKENS` | `--openai-max-tokens` | - | `32000` | Maximum tokens for completions |
| **Logging** |
| Log level | `LOG_LEVEL` | `--log-level` | - | `info` | Logging level |

### Example configuration

```bash
# .env file
THEHIVE_URL=https://thehive.example.com
THEHIVE_API_KEY=<thehive_api_key>
THEHIVE_ORGANISATION=<thehive_organization>
PERMISSIONS_CONFIG=docs/examples/permissions/analyst.yaml  # Optional, defaults to read-only
MCP_BIND_HOST=0.0.0.0
MCP_PORT=8082
OPENAI_API_KEY=<openai_api_key>  # Optional, for fallback LLM
LOG_LEVEL=INFO
```

**Multi-tenant:** Override configuration per-request using `Authorization`, `X-TheHive-Org`, and `X-TheHive-Url` headers.

**Permissions:** Control tool access and data filtering. See [docs/permissions.md](docs/permissions.md) for detailed configuration.

</details>

<details>
<summary><strong>🚀 Advanced features</strong></summary>

## Advanced Features

### MCP sampling

Natural language queries in `search-entities` require an LLM. TheHiveMCP uses client-side sampling if available, otherwise falls back to server-side OpenAI. Configure `OPENAI_API_KEY` for fallback support. Without either, natural language search fails (other tools work normally).

### MCP elicitation

Modifying operations (create, update, delete) request user confirmation if the client supports elicitation. Without support, operations proceed automatically.

</details>

## Deployment Options

### Standalone server (HTTP or Stdio)

Standard deployment runs TheHiveMCP as a standalone server process. See [Installation](#installation) section.

### In-process integration

Embed TheHiveMCP into Go applications using the `bootstrap` package:

```go
import "github.com/StrangeBeeCorp/TheHiveMCP/bootstrap"

// Use environment credentials
mcpServer := bootstrap.GetMCPServerAndRegisterTools()

// Or use custom credentials with permissions
creds := &bootstrap.TheHiveCredentials{
    URL: "https://thehive.example.com", APIKey: "key", Organisation: "org",
}
// Second parameter is permissions config path ("" = read-only default)
mcpServer := bootstrap.GetInprocessServer(creds, "/path/to/permissions.yaml")
bootstrap.RegisterToolsToMCPServer(mcpServer)
```

**Note:** Only the `bootstrap` package is public API. Internal packages may change without notice.

## MCP Tools

- **search-entities**: Search for entities using natural language (for example, "high severity alerts from last week")
- **manage-entities**: Create, update, delete entities, add comments
- **execute-automation**: Run Cortex analyzers and responders, check job status
- **get-resource**: Access schemas, docs, and metadata through hierarchical browsing (for example, `uri="hive://schema"` or `uri="hive://metadata/automation"`)

<details>
<summary><strong>🔧 Detailed tool documentation</strong></summary>

### [get-resource](docs/tools/get-resource.md)
Access TheHive resources for documentation, schemas, and metadata. The entry point for exploring TheHive capabilities through a hierarchical URI-based resource system.

**Key features:**
- Browse resource catalog and categories with flexible navigation
- Access entity schemas (output, create, and update variants for each entity type)
- Query metadata for available options with subcategory support
- Get comprehensive documentation through hierarchical paths

**Schema organization:**
- Output schemas: `hive://schema/{entity}` - Fields returned from queries
- Create schemas: `hive://schema/{entity}/create` - Required fields for creation
- Update schemas: `hive://schema/{entity}/update` - Available fields for updates

**Navigation examples:**
- Browse automation metadata: `uri="hive://metadata/automation"`
- List entity schemas: `uri="hive://schema"`
- Get specific alert schema: `uri="hive://schema/alert"`

### [search-entities](docs/tools/search-entities.md)
Search for entities in TheHive using natural language queries. Uses AI to translate natural language into TheHive filters.

**Key features:**
- Natural language query processing
- Support for all entity types (alerts, cases, tasks, observables)
- Flexible filtering and sorting options
- Custom column and data field selection

### [manage-entities](docs/tools/manage-entities.md)
Perform comprehensive CRUD operations on TheHive entities with full support for relationships and constraints.

**Key features:**
- Create, update, delete operations for all entity types
- Comment support for cases and task logs
- Respect for entity hierarchies and relationships
- Batch operations support

### [execute-automation](docs/tools/execute-automation.md)
Execute Cortex analyzers and responders with comprehensive status monitoring and parameter customization.

**Key features:**
- Run analyzers on observables for threat intelligence
- Execute responders for automated actions
- Monitor job and action status
- Support for custom parameters and multiple Cortex instances

</details>

## MCP Resources

Static resources include entity schemas (with separate output, create, and update variants) and documentation. Dynamic resources provide live data (users, templates, analyzers, responders, observable types).

<details>
<summary><strong>🔨 Development</strong></summary>

## Development

### Dockerized development workflow

All development operations use Docker containers for consistency and isolation:

**Core commands:**
- `make all` - Format, security checks, tests, and build
- `make build` - Build binary using Docker
- `make run ARGS="arguments"` - Run application with custom arguments
- `make test` - Run tests with Docker network support for integration tests
- `make dev` - Development server with hot reload (requires local air)

**Quality and security:**
- `make fmt` - Format code using Docker
- `make security` - Run all security checks (vulncheck, sast, vetlint)
- `make sast` - Static application security testing
- `make vetlint` - Linting checks
- `make vulncheck` - Vulnerability scanning

**Docker operations:**
- `make docker-build` - Build production Docker image
- `make docker-run` - Run production container

**Dependencies:**
- `make updatedep` - Update Go dependencies
- `make install-dev-deps` - Install development tools (Docker-based)

**Utilities:**
- `make clean` - Remove build artifacts
- `make help` - Display all available targets

**Architecture:** Transport (`bootstrap/`), Tools (`internal/tools/`), Resources (`internal/resources/`), Integration (`internal/utils/`), Prompts (`internal/prompts/`)

</details>

## License

This project is licensed under the AGPL-3.0 License.

## Related Projects

- [TheHive](https://github.com/TheHive-Project/TheHive) - Security Incident Response Platform
- [thehive4go](https://github.com/StrangeBeeCorp/thehive4go) - TheHive Go SDK
- [mcp-go](https://github.com/mark3labs/mcp-go) - Model Context Protocol Go SDK
- [Hivemind](https://github.com/StrangeBeeCorp/hivemind) - Previous generation MCP server

---

Open source project maintained by StrangeBee. [Issues and contributions welcome](https://github.com/StrangeBeeCorp/TheHiveMCP/issues).

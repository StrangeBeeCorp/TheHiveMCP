#!/bin/bash
set -e

# CI Mode Detection
CI_MODE=${CI_MODE:-false}
WORKSPACE_DIR=${WORKSPACE_DIR:-$(pwd)}

# Load environment variables from .env file (if not in CI)
if [ "$CI_MODE" != "true" ] && [ -f "./.env" ]; then
    export $(grep -v '^#' ./.env | xargs)
fi

# Set defaults for OpenAI variables if not already set
export OPENAI_API_KEY="${OPENAI_API_KEY:-}"
export OPENAI_BASE_URL="${OPENAI_BASE_URL:-https://openrouter.ai/api/v1}"
export OPENAI_MODEL="${OPENAI_MODEL:-anthropic/claude-haiku-4.5}"

# Set default for permissions config
export PERMISSIONS_CONFIG="${PERMISSIONS_CONFIG:-}"

# Check if permissions config is a file before changing directories
PERMISSIONS_IS_FILE=false
if [ -n "$PERMISSIONS_CONFIG" ] && [ "$PERMISSIONS_CONFIG" != "TESTING_ADMIN" ]; then
    # Check if file exists (handling both absolute and relative paths)
    if [ -f "$PERMISSIONS_CONFIG" ]; then
        PERMISSIONS_IS_FILE=true
    fi
fi

# Create extension directory structure
mkdir -p extension/server
cd extension

# Copy logo - handle both CI and local modes
if [ "$CI_MODE" = "true" ]; then
    cp /usr/local/share/icon.png icon.png
else
    cp ../docs/images/theHivelogo.png icon.png
fi

# Copy permissions config if it's a file path and bundle it
PERMISSIONS_DEFAULT=""
if [ "$PERMISSIONS_IS_FILE" = true ]; then
    # File exists, copy it to the bundle
    if [ "$CI_MODE" = "true" ]; then
        cp "$PERMISSIONS_CONFIG" permissions.yaml
    else
        # Try relative path from project root first, then absolute/current path
        if ! cp ../"$PERMISSIONS_CONFIG" permissions.yaml 2>/dev/null && ! cp "$PERMISSIONS_CONFIG" permissions.yaml 2>/dev/null; then
            echo "Error: Failed to copy permissions config from '$PERMISSIONS_CONFIG'" >&2
            echo "Tried paths: ../$PERMISSIONS_CONFIG and $PERMISSIONS_CONFIG" >&2
            exit 1
        fi
    fi
    PERMISSIONS_DEFAULT="permissions.yaml"
    echo "Bundled permissions config: $PERMISSIONS_CONFIG -> permissions.yaml"
elif [ -n "$PERMISSIONS_CONFIG" ]; then
    # Not a file (empty string, TESTING_ADMIN, etc), use as-is
    PERMISSIONS_DEFAULT="$PERMISSIONS_CONFIG"
fi

# Handle binary selection - in CI we'll build for all platforms
if [ "$CI_MODE" = "true" ]; then
    # In CI, expect binaries to be provided in /workspace/binaries/
    # Use TARGET_ARCH if specified, otherwise default to linux-amd64
    TARGET_ARCH=${TARGET_ARCH:-linux-amd64}
    BINARY_NAME="thehivemcp-${TARGET_ARCH}"
    if [ -f "/workspace/binaries/$BINARY_NAME" ]; then
        cp /workspace/binaries/$BINARY_NAME server/thehivemcp
    else
        echo "Error: Binary $BINARY_NAME not found in /workspace/binaries/" >&2
        exit 1
    fi
else
    # Local mode: detect platform and copy appropriate binary
    PLATFORM=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)
    if [ "$ARCH" = "x86_64" ]; then
        ARCH="amd64"
    fi
    BINARY_NAME="thehivemcp-${PLATFORM}-${ARCH}"
    cp ../build/$BINARY_NAME server/thehivemcp
fi

chmod +x server/thehivemcp

# Extract version - handle both CI and local modes
if [ "$CI_MODE" = "true" ]; then
    VERSION_FULL=${VERSION:-$(echo "$BINARY_NAME" | grep -o 'v[0-9]\+\.[0-9]\+\.[0-9]\+' || echo "v0.0.0")}
    VERSION=$(echo "$VERSION_FULL" | sed 's/^v//')
else
    VERSION_FULL=$(cd .. && make version)
    VERSION=$(echo "$VERSION_FULL" | sed 's/^v//')
fi

cat > manifest.json << EOF
{
  "manifest_version": "0.2",
  "name": "TheHiveMCP",
  "version": "$VERSION",
  "description": "Connect to TheHive and Cortex via Hivemind MCP for security incident management",
  "long_description": "StrangeBee Hivemind MCP enables Claude to interact with TheHive security incident response platform and Cortex analysis engine...",
  "author": {
    "name": "StrangeBee",
    "email": "admin@strangebee.com",
    "url": "https://strangebee.com"
  },
  "homepage": "https://docs.strangebee.com/mcp",
  "icon": "icon.png",
  "server": {
    "type": "binary",
    "entry_point": "server/thehivemcp",
    "mcp_config": {
      "command": "\${__dirname}/server/thehivemcp",
      "args": ["-transport", "stdio"],
      "env": {
        "MCP_PORT": "8082",
        "THEHIVE_URL": "\${user_config.thehive_url}",
        "THEHIVE_API_KEY": "\${user_config.thehive_api_key}",
        "THEHIVE_ORGANISATION": "\${user_config.organisation}",
        "PERMISSIONS_CONFIG": "\${user_config.permissions_config}",
        "OPENAI_API_KEY": "\${user_config.openai_api_key}",
        "OPENAI_BASE_URL": "\${user_config.openai_base_url}",
        "OPENAI_MODEL": "\${user_config.openai_model}"
      }
    }
  },
  "user_config": {
    "thehive_url": {
      "title": "TheHive URL",
      "description": "Your TheHive instance URL (e.g., https://thehive.company.com)",
      "type": "string",
      "required": true
    },
    "thehive_api_key": {
      "title": "TheHive API Key",
      "description": "Your TheHive API key for authentication",
      "type": "string",
      "required": true,
      "sensitive": true
    },
    "organisation": {
      "title": "Organisation",
      "description": "TheHive organisation name",
      "type": "string",
      "required": true
    },
    "permissions_config": {
      "title": "Permissions Config",
      "description": "Path to permissions YAML config file (leave empty for read-only default, or use bundled relative path)",
      "type": "string",
      "required": false,
      "default": "$PERMISSIONS_DEFAULT"
    },
    "openai_api_key": {
      "title": "OpenAI API Key",
      "description": "OpenAI/OpenRouter API key for AI features",
      "type": "string",
      "required": true,
      "sensitive": true,
      "default": "$OPENAI_API_KEY"
    },
    "openai_base_url": {
      "title": "OpenAI Base URL",
      "description": "OpenAI API base URL (use OpenRouter for model variety)",
      "type": "string",
      "required": false,
      "default": "$OPENAI_BASE_URL"
    },
    "openai_model": {
      "title": "OpenAI Model",
      "description": "AI model to use for analysis and automation",
      "type": "string",
      "required": false,
      "default": "$OPENAI_MODEL"
    }
  }
}
EOF

# Pack the MCPB
echo "Packaging MCPB with version: $VERSION"
npx @anthropic-ai/mcpb pack

# In CI mode, move the generated file to expected location
if [ "$CI_MODE" = "true" ]; then
    # Find the generated .mcpb file and copy it to workspace with architecture suffix
    MCPB_FILE=$(find . -name "*.mcpb" -type f | head -1)
    if [ -n "$MCPB_FILE" ]; then
        OUTPUT_NAME="/workspace/thehivemcp-${VERSION_FULL}-${TARGET_ARCH}.mcpb"
        cp "$MCPB_FILE" "$OUTPUT_NAME"
        echo "MCPB package created: $OUTPUT_NAME"
    else
        echo "Error: No .mcpb file generated" >&2
        exit 1
    fi
fi

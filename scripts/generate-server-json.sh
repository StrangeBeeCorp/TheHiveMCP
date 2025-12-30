#!/bin/bash

# Extract version from git tag, env var, or argument
if [ -n "$1" ]; then
    VERSION="$1"
elif [ -n "$GITHUB_REF_NAME" ]; then
    VERSION="$GITHUB_REF_NAME"
else
    VERSION=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
fi

VERSION_NUMBER="${VERSION#v}"
BUILD_DIR="${2:-./build}"
REPO="${GITHUB_REPOSITORY:-StrangeBeeCorp/TheHiveMCP}"

echo "Generating server.json for $REPO version $VERSION"

cat > server.json << EOF
{
  "\$schema": "https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json",
  "name": "io.github.${REPO}",
  "description": "MCP server for TheHive security platform - AI-powered incident response",
  "version": "${VERSION_NUMBER}",
  "homepage": "https://github.com/${REPO}",
  "license": "MIT",
  "packages": [
EOF

PLATFORMS=("darwin-amd64" "darwin-arm64" "linux-amd64" "linux-arm64" "windows-amd64")
found=0

for i in "${!PLATFORMS[@]}"; do
    platform="${PLATFORMS[$i]}"
    filename="thehivemcp-${VERSION}-${platform}.mcpb"
    filepath="${BUILD_DIR}/${filename}"

    if [ -f "$filepath" ]; then
        hash=$(openssl dgst -sha256 "$filepath" | cut -d' ' -f2)
        url="https://github.com/${REPO}/releases/download/${VERSION}/${filename}"

        echo "    {" >> server.json
        echo "      \"registryType\": \"mcpb\"," >> server.json
        echo "      \"identifier\": \"${url}\"," >> server.json
        echo "      \"version\": \"${VERSION_NUMBER}\"," >> server.json
        echo "      \"fileSha256\": \"${hash}\"," >> server.json
        echo "      \"transport\": {" >> server.json
        echo "        \"type\": \"stdio\"" >> server.json
        echo "      }" >> server.json
        echo "    }," >> server.json

        found=$((found + 1))
        echo "  ✓ Added ${filename}"
    else
        echo "  ⚠ Warning: ${filepath} not found" >&2
    fi
done

if [ $found -eq 0 ]; then
    echo "Error: No .mcpb files found in ${BUILD_DIR}" >&2
    exit 1
fi

# Add Docker package
DOCKER_REGISTRY="${DOCKER_REGISTRY:-docker.io}"
DOCKER_IMAGE="${DOCKER_IMAGE:-strangebee/thehive-mcp}"

cat >> server.json << EOF
    {
      "registryType": "oci",
      "registryBaseUrl": "https://${DOCKER_REGISTRY}",
      "identifier": "${DOCKER_IMAGE}",
      "version": "${VERSION_NUMBER}",
      "transport": {
        "type": "stdio"
      }
    }
  ]
}
EOF

echo "✓ Generated server.json with ${found} MCPB packages + 1 OCI package"

#!/bin/bash

# Extract version from git tag or use argument
if [ -n "$1" ]; then
    VERSION="$1"
elif [ -n "$GITHUB_REF_NAME" ]; then
    VERSION="$GITHUB_REF_NAME"
else
    VERSION=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
fi

BUILD_DIR="${2:-./dist}"

echo "Generating SHA256 hashes for TheHiveMCP $VERSION"
echo ""

found=0
for file in $BUILD_DIR/thehivemcp-$VERSION-*.mcpb; do
    if [ -f "$file" ]; then
        filename=$(basename "$file")
        hash=$(openssl dgst -sha256 "$file" | cut -d' ' -f2)
        echo "File: $filename"
        echo "SHA256: $hash"
        echo ""
        found=$((found + 1))
    fi
done

if [ $found -eq 0 ]; then
    echo "Error: No .mcpb files found in $BUILD_DIR for version $VERSION" >&2
    exit 1
fi

echo "Total files: $found"

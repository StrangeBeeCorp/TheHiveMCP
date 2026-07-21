#!/usr/bin/env bash
# Wrap the already-built (and, in release CI, already-signed) binaries in
# $BUILDDIR into MCPB packages under $DISTDIR, one per target, via the pinned
# thehivemcp-mcpb:latest image. Extracted from the Makefile's `mcpb-ci-nobuild`
# target; it deliberately does NOT build, so a signing step can run between
# build-all and packaging.
#
# All inputs come from the environment (set by the Makefile):
#   VERSION       release version string baked into package names
#   BUILDDIR      directory holding the built binaries
#   DISTDIR       output directory (must already exist; `pre-dist` creates it)
#   MCPB_TARGETS  space-separated GOOS-GOARCH targets to package
#                 (a subset narrows the run, e.g. the mcpb-smoke one-arch check)
set -euo pipefail

: "${VERSION:?}" "${BUILDDIR:?}" "${DISTDIR:?}" "${MCPB_TARGETS:?}"

for target in $MCPB_TARGETS; do
  echo "Generating MCPB for $target..."
  case "$target" in windows-*) ext=".exe" ;; *) ext="" ;; esac
  ws="/tmp/mcpb-workspace-$target"
  mkdir -p "$ws/binaries"
  cp "$BUILDDIR/thehivemcp-$target$ext" "$ws/binaries/"
  # Run as the host uid:gid so files written to the $ws volume are owned by the
  # runner user, not root (see scripts/generate-mcpb.sh for the HOME=/workspace
  # rationale that pairs with this).
  docker run --rm \
    --user "$(id -u):$(id -g)" \
    -v "$ws:/workspace" \
    -e HOME=/workspace \
    -e CI_MODE=true \
    -e VERSION="$VERSION" \
    -e TARGET_ARCH="$target" \
    thehivemcp-mcpb:latest
  cp "$ws/thehivemcp-$VERSION-$target.mcpb" "$DISTDIR/"
  shasum -a 256 "$DISTDIR/thehivemcp-$VERSION-$target.mcpb" \
    >"$DISTDIR/thehivemcp-$VERSION-$target.mcpb.sha256"
  # Clean the root-owned files the container may have left, falling back to a
  # plain rm; never fail the packaging run on cleanup.
  docker run --rm -v "$ws:/workspace" alpine:latest rm -rf /workspace/* || rm -rf "$ws" || true
done

echo "All MCPB packages created in $DISTDIR/"

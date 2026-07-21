#!/usr/bin/env bash
# Package the already-built binaries in $BUILDDIR into per-target .tar.gz archives
# (plus a .sha256 each) under $DISTDIR. Extracted from the Makefile's
# `package-from-built` target; it packages whatever binaries already exist and
# does NOT (re)build, so a signing step can run between build-all and packaging.
#
# All inputs come from the environment (set by the Makefile):
#   VERSION          release version string baked into archive names
#   BUILDDIR         directory holding the built binaries
#   DISTDIR          output directory (must already exist; `pre-dist` creates it)
#   BINARY_NAME      base binary name (e.g. thehivemcp)
#   RELEASE_TARGETS  space-separated GOOS-GOARCH targets to package
set -euo pipefail

: "${VERSION:?}" "${BUILDDIR:?}" "${DISTDIR:?}" "${BINARY_NAME:?}" "${RELEASE_TARGETS:?}"

for target in $RELEASE_TARGETS; do
  echo "Packaging $target..."
  case "$target" in windows-*) ext=".exe" ;; *) ext="" ;; esac
  cp "$BUILDDIR/$BINARY_NAME-$target$ext" "$DISTDIR/$BINARY_NAME-$target$ext"
  (cd "$DISTDIR" && tar -czf "$BINARY_NAME-$VERSION-$target.tar.gz" "$BINARY_NAME-$target$ext")
  shasum -a 256 "$DISTDIR/$BINARY_NAME-$VERSION-$target.tar.gz" \
    >"$DISTDIR/$BINARY_NAME-$VERSION-$target.tar.gz.sha256"
  rm "$DISTDIR/$BINARY_NAME-$target$ext"
done

echo "All release packages created in $DISTDIR/"
ls -1 "$DISTDIR/"

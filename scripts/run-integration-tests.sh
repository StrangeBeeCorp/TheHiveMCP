#!/usr/bin/env bash
# Run the full integration suite against a live TheHive stack. Extracted from the
# Makefile's `test-integration` target.
#
# Brings up the TheHive + Elasticsearch + Cassandra + MITRE stack (THEHIVE_TEST_IMAGE
# selects the version) and resets its databases to a clean state (via
# reset-integration-db.sh — see it for the why), then runs the suite against it on
# the host network. The stack is deliberately LEFT UP for fast reruns;
# `make test-integration-down` tears it down.
#
# The reset script decides the mode (presupplied THEHIVE_TEST_LICENSE, else a
# pullable image ⇒ mint, else free) and writes the token (or nothing) to
# LICENSE_FILE. We read that file here to configure the go-test container:
#   - Non-empty (token present) ⇒ export THEHIVE_TEST_LICENSE so the Go suite runs
#     per-test org + user via t.Parallel (testutils.Parallel), packages concurrent
#     (no -p 1).
#   - Empty/absent (free license) ⇒ one shared org, testutils.Parallel no-ops so
#     tests run sequentially and purge their own data; `-p 1` stops packages from
#     racing on that shared org.
# Isolation in either mode comes from internal/testutils/orgs.go.
#
# -timeout 20m raises the per-package deadline above the default 10m: under memory
# pressure a single request can stall, and the setup helpers retry those (each
# capped at the client's 90s transport timeout), so the headroom keeps a transient
# stall from tripping the package timeout. The sequential free-license path is the
# slowest run, so it needs this headroom most.
#
# All inputs come from the environment (set by the Makefile):
#   LICENSE_FILE        path reset-integration-db.sh writes the license token to
#   MOUNT_APP           `$(CURDIR):/app` bind mount for the go-test container
#   GO_RUN_AS_HOST_UID  uid/env flags for CI cache ownership (may be empty)
#   DOCKER_CACHE_MOUNTS Go module/build cache volume flags
#   GO_TOOLS_CACHE      go-installed-tools cache volume flag
#   GO_IMAGE            the Go container image
#   GO_TEST_COVER       coverage flags (may be empty)
#   GO_TEST_RUN         -run regexp flag (may be empty)
# THEHIVE_TEST_URL, LOG_LEVEL, THEHIVE_TEST_IMAGE are passed through to the
# container from the ambient environment.
set -euo pipefail

: "${LICENSE_FILE:?}" "${MOUNT_APP:?}" "${GO_IMAGE:?}"

./scripts/reset-integration-db.sh docker-compose.test.yml

if [[ -s "$LICENSE_FILE" ]]; then
  echo "License present → parallel multi-org mode."
  LIC="$(cat "$LICENSE_FILE")"
  PARALLELISM=""
else
  echo "No license → sequential shared-org mode."
  LIC=""
  PARALLELISM="-p 1"
fi

# shellcheck disable=SC2086 # cache/uid var groups are intentionally word-split into flags
docker run -i --rm --network host -v "$MOUNT_APP" -w /app \
  -e THEHIVE_TEST_URL -e LOG_LEVEL -e THEHIVE_TEST_IMAGE -e THEHIVE_TEST_LICENSE="$LIC" \
  ${GO_RUN_AS_HOST_UID:-} ${DOCKER_CACHE_MOUNTS:-} ${GO_TOOLS_CACHE:-} "$GO_IMAGE" \
  sh -c "command -v gotestsum >/dev/null 2>&1 || go install gotest.tools/gotestsum@v1.13.0 ; gotestsum --format pkgname --hide-summary=skipped -- ${GO_TEST_COVER:-} ${GO_TEST_RUN:-} $PARALLELISM -timeout 20m ./..."

#!/bin/bash
#
# Bring the integration test stack up and reset its databases to a clean state,
# leaving TheHive ready for a fresh test run. Invoked by `make test-integration`.
#
# Why reset instead of `down -v` between runs: the cold boot is dominated by
# Cassandra (~60s) plus TheHive's schema bootstrap, so tearing the stack down
# every time makes repeated local runs slow. We instead keep the stack UP and
# wipe the data at the start of each run — DROP the JanusGraph keyspace and
# delete TheHive's Elasticsearch indices, then restart TheHive so it
# re-bootstraps a fresh schema (~15s while Cassandra stays warm).
#
# This must be hermetic: the per-test org/user counters (internal/testutils/
# orgs.go) restart from 0 in every test process, so a dirty DB would make the
# next run reuse orgs that still hold the previous run's data and break the
# suite's count assertions (require.Len / require.Equal on row counts).
#
# Mode selection (see internal/testutils/orgs.go for the matching Go-side
# branch). The single gate is whether the StrangeBee licensing image is
# PULLABLE:
#   - Pullable (private-repo CI / an authenticated dev): boot TheHive in --dev
#     mode, MINT a multi-instance dev license on the fly, activate it, and
#     write it to the gitignored .test-license.lic.local that the license
#     override mounts. The suite then runs parallel with one org per test
#     (unlimited-org quota). No secret is committed or configured anywhere —
#     the token is minted per run.
#   - Not pullable (public mirror / external contributors): the base file alone
#     boots TheHive on its built-in free license (1 org); the suite runs
#     sequentially against one shared org.
# The Makefile reads .test-license.lic.local after this script returns: a
# non-empty file ⇒ license (parallel) mode, absent/empty ⇒ free (sequential).
#
# On any failure here (before the tests run) the stack is torn down via a trap,
# so a half-started or half-reset stack is never left behind. On success the
# stack is deliberately left UP; `make test-integration-down` tears it down.

set -euo pipefail

COMPOSE_FILE="${1:-docker-compose.test.yml}"
LICENSE_FILE="internal/testutils/testdata/.test-license.lic.local"

# The licensing image registry path. A key minted by any recent tag validates
# against 5.5 / 5.6 / 5.7 servers (the license format is version-stable), so we
# probe the tag derived from THEHIVE_TEST_IMAGE first, then fall back to a
# known-published tag. Whichever is pullable is used to mint.
LICENSING_REPO="ghcr.io/strangebee/tools/licensing"
LICENSING_FALLBACK_TAG="release-5.6"

# derive_licensing_tag maps THEHIVE_TEST_IMAGE (e.g. strangebee/thehive:5.6.3)
# to a licensing tag (release-5.6). Empty if no version can be parsed.
derive_licensing_tag() {
	local img="${THEHIVE_TEST_IMAGE:-}"
	local ver majmin
	ver="${img##*:}" # 5.6.3
	majmin="$(echo "$ver" | grep -oE '^[0-9]+\.[0-9]+' || true)"
	[ -n "$majmin" ] && echo "release-${majmin}"
}

# pick_licensing_image echoes the first pullable "<repo>:<tag>" among the
# derived tag and the fallback, or nothing if none is pullable.
pick_licensing_image() {
	local tag
	for tag in "$(derive_licensing_tag)" "$LICENSING_FALLBACK_TAG"; do
		[ -n "$tag" ] || continue
		if docker manifest inspect "${LICENSING_REPO}:${tag}" >/dev/null 2>&1; then
			echo "${LICENSING_REPO}:${tag}"
			return 0
		fi
	done
	return 0
}

LICENSING_IMAGE="$(pick_licensing_image)"

# Start from a clean license file every run: whether we mint below or not, a
# stale token from a previous run must never leak into a free-mode run.
rm -f "${LICENSE_FILE}"

if [ -n "${LICENSING_IMAGE}" ]; then
	echo "Licensing image ${LICENSING_IMAGE} is pullable → license (parallel) mode."
	# The override mounts LICENSE_FILE; create an empty placeholder so the mount
	# source exists and TheHive boots in --dev mode (unlicensed-but-dev) for the
	# minting challenge below.
	: >"${LICENSE_FILE}"
	DC="docker compose -f ${COMPOSE_FILE} -f docker-compose.license.yml"
else
	echo "Licensing image not pullable → free (sequential) mode."
	DC="docker compose -f ${COMPOSE_FILE}"
fi

# Tear the stack down if we fail before handing off to the test run. Cleared on
# success so the stack stays up for fast reruns / inspection.
cleanup() {
	status=$?
	echo "reset failed (status ${status}); tearing the stack down" >&2
	${DC} down -v || true
	exit "${status}"
}
trap cleanup EXIT INT TERM

# `up -d` starts the stack if it is not running and is a no-op otherwise. It
# blocks until Cassandra is healthy (compose `depends_on: service_healthy`).
${DC} up -d

# Mint the license against the freshly-booted --dev server. The token is
# multi-instance, so it survives the keyspace DROP + restart below (which gives
# the DB a new instance id). We also write it to LICENSE_FILE so a restart
# re-activates it from disk, and so the Makefile can pass it to the go-test
# container to select parallel mode.
if [ -n "${LICENSING_IMAGE}" ]; then
	echo "Waiting for TheHive to accept the licensing challenge…"
	for _ in $(seq 1 72); do
		code="$(curl -s -o /dev/null -w '%{http_code}' http://localhost:9000/api/status || true)"
		[ "${code}" = "200" ] && break
		sleep 5
	done

	echo "Minting a multi-instance dev license…"
	mint_out="$(docker run --network host --rm -i "${LICENSING_IMAGE}" \
		--key-id dev \
		--thehive http://admin:secret@localhost:9000 \
		--expiration 365days \
		--customer StrangeBee \
		--plan Platinum \
		--multiInstance \
		--quotas users.normal=-1,organisations=-1 \
		--stdout 2>&1)"

	# The tool prints logs + "Token: <jwt>" + "Setting license … OK". Extract the
	# JWT line; fail loudly if it is missing (so we never silently fall back to a
	# free run when the operator expected parallel).
	token="$(printf '%s\n' "${mint_out}" | sed -n 's/^Token: //p' | head -1)"
	if [ -z "${token}" ]; then
		echo "Failed to mint license; tool output was:" >&2
		printf '%s\n' "${mint_out}" >&2
		exit 1
	fi
	printf '%s' "${token}" >"${LICENSE_FILE}"
	echo "License minted and activated."
fi

echo "Resetting databases (DROP keyspace + delete ES indices + restart TheHive)…"

# DROP the graph keyspace (IF EXISTS: absent on the very first run against a
# fresh stack) and delete TheHive's ES indices (glob; ignore 404 when absent).
${DC} exec -T cassandra cqlsh -e "DROP KEYSPACE IF EXISTS thehive;"
${DC} exec -T elasticsearch sh -c 'curl -s -o /dev/null -X DELETE "http://localhost:9200/thehive*"'

# Restart TheHive so JanusGraph re-creates the schema against the now-empty
# keyspace (it only bootstraps the schema at startup, not at runtime). In
# license mode this also re-activates the on-disk token against the new DB
# instance. The Go suite waits for readiness itself
# (testutils.StartTheHiveContainer), so we do not need to poll here.
${DC} restart thehive

# Hand off to the caller (the test run) with the stack up and clean.
trap - EXIT INT TERM
echo "Stack is up and reset; ready for the test run."

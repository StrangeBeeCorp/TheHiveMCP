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
# On any failure here (before the tests run) the stack is torn down via a trap,
# so a half-started or half-reset stack is never left behind. On success the
# stack is deliberately left UP; `make test-integration-down` tears it down.

set -euo pipefail

COMPOSE_FILE="${1:-docker-compose.test.yml}"
DC="docker compose -f ${COMPOSE_FILE}"

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

echo "Resetting databases (DROP keyspace + delete ES indices + restart TheHive)…"

# DROP the graph keyspace (IF EXISTS: absent on the very first run against a
# fresh stack) and delete TheHive's ES indices (glob; ignore 404 when absent).
${DC} exec -T cassandra cqlsh -e "DROP KEYSPACE IF EXISTS thehive;"
${DC} exec -T elasticsearch sh -c 'curl -s -o /dev/null -X DELETE "http://localhost:9200/thehive*"'

# Restart TheHive so JanusGraph re-creates the schema against the now-empty
# keyspace (it only bootstraps the schema at startup, not at runtime). The Go
# suite waits for readiness itself (testutils.StartTheHiveContainer), so we do
# not need to poll here.
${DC} restart thehive

# Hand off to the caller (the test run) with the stack up and clean.
trap - EXIT INT TERM
echo "Stack is up and reset; ready for the test run."

# Develop and test against a shared TheHive

By default, everything in this repository that needs a TheHive starts its own: `make test-integration` boots a disposable TheHive + Cassandra + Elasticsearch
stack from `docker-compose.test.yml`, resets its database and mints a licence. That stays the default (CI and every contributor rely on it). It is also heavy, a
few GB of memory, and slow to boot.

If you already have a long-running TheHive that several projects share, you can instead point the dev server and the integration suite at **one organisation of
that instance**. Nothing in this guide is needed for the default workflow.

## What shared mode does and does not do

Shared mode (`THEHIVE_TEST_SHARED=1`) runs the integration suite inside one organisation, authenticated with the API key of an org-admin of that organisation.
Compared with the default mode:

- It never uses the platform admin, never creates organisations or users, and never changes anything instance-wide.
- It does not boot `docker-compose.test.yml`, reset the database or mint a licence.
- The tests run one after another in that organisation (like the free-licence mode), and the organisation is purged (cases, alerts, case templates, pages) after
  every test. **Use an organisation dedicated to the tests**: its data is deleted.
- Tests that need instance-wide data are skipped with a message instead of failing. Today that is the ATT&CK pattern `T1059`, used by the procedure tests
  (`TestManageCreateProcedureInCase`, `TestManageUpdateProcedure`, `TestManageDeleteProcedure`) and `TestSearchPatternsCarryNoUntrustedFence`. The ATT&CK
  catalogue is shared by every organisation, so the suite does not import it; the owner of the instance does, once (see below).
- TheHive-version-specific testing (5.5, 5.8 and 6.0) stays in CI, where the matrix boots each version. In shared mode you test against whatever version the
  shared instance runs.

## Run the integration suite in shared mode

You need the instance URL, the name of an organisation and a file holding the API key of one of its org-admins.

```sh
THEHIVE_TEST_URL=https://thehive.example.com \
THEHIVE_TEST_ORG=mcp-integration-tests \
THEHIVE_TEST_API_KEY_FILE=~/path/to/org-admin-key \
make test-integration-shared RUN=TestSearch   # RUN is optional; quote a regexp with "|": RUN="'TestA|TestB'"
```

`make test-integration-shared` is `THEHIVE_TEST_SHARED=1 make test-integration`: `bin/harness integration` (`internal/harness/stack`) resolves the target
instead of starting the compose stack, and the suite is still compiled and run in the pinned Go container, which must be able to reach the URL.

| Variable                    | Meaning                                                                                 |
| --------------------------- | --------------------------------------------------------------------------------------- |
| `THEHIVE_TEST_URL`          | Base URL of the shared TheHive                                                          |
| `THEHIVE_TEST_ORG`          | Organisation the tests run and purge in                                                 |
| `THEHIVE_TEST_API_KEY_FILE` | File holding that organisation's org-admin API key (or `THEHIVE_TEST_API_KEY`, the key) |
| `THEHIVE_TEST_SHARED`       | `1` selects shared mode (set by `make test-integration-shared`)                         |

### With devstack

On a machine that runs `devstack` (a local broker that shares one TheHive between projects and queues heavy jobs), the URL, organisation and key are optional:
when no key is given and `devstack` is installed, `bin/harness integration` leases the organisation `thehivemcp-it` (override with `THEHIVE_TEST_ORG`), uses its
URL and key, and releases the lease when the run ends, whether the suite passed, failed or was interrupted. Because the run is heavy, go through the broker:

```sh
devstack run --mem 3000 -- make test-integration-shared RUN=TestSearch
```

The ATT&CK pattern is imported once into the shared instance with the spec shipped in the repository:

```sh
devstack instance apply internal/testutils/testdata/devstack-instance.json
```

The spec only points at `internal/testutils/testdata/mitre.json`, the one-technique bundle the default stack imports. Until it is applied, the pattern tests
skip.

## Run the MCP server locally against the shared TheHive

The server takes its target from `THEHIVE_URL`, `THEHIVE_ORGANISATION` and `THEHIVE_API_KEY` (or the matching flags). Use the organisation's org-admin key (or
any key of a narrower user of that organisation):

```sh
export THEHIVE_URL=https://thehive.example.com
export THEHIVE_ORGANISATION=my-dev-org
export THEHIVE_API_KEY="$(cat ~/path/to/org-admin-key)"
make run ARGS="--transport http --addr 127.0.0.1:8082"      # or: --transport stdio
```

With devstack, get the values from a lease (the key stays in a file, never in the command line):

```sh
devstack lease thehive --org my-dev-org --for "TheHiveMCP dev server"   # JSON: thehive_url, org, token_file, release
export THEHIVE_URL=http://thehive.devstack.orb.local THEHIVE_ORGANISATION=my-dev-org
export THEHIVE_API_KEY="$(cat <token_file from the lease>)"
make run ARGS="--transport http --addr 127.0.0.1:8082"
devstack release <lease>                                                 # when done
```

devstack also runs a shared MCP server, `http://mcp.devstack.orb.local/mcp` (latest release), that takes its credentials per request (`X-TheHive-Org` and
`X-TheHive-Api-Key` headers): use it to try the released server without running anything.

`make dev` (hot reload with `air`) reads the same variables; it needs a Go toolchain on the host.

## Default mode: another superadmin login

In the default mode the Go suite logs in to the compose stack as the built-in superadmin (`admin@thehive.local` / `secret`). If your stack's superadmin differs,
set `THEHIVE_TEST_ADMIN_USER` and `THEHIVE_TEST_ADMIN_PASSWORD`; unset, those defaults apply. Shared mode ignores both.

# Tests in TheHiveMCP

## Unit or integration: the helper decides

- `testutils.StartTheHiveContainer` calls `t.Skip` under `-short`. Every test that needs a live TheHive reaches it, either directly or through
  `SetupTestWithCleanup` or `GetMCPTestClient` / `GetMCPTestClientWithPermissions`. So `make test` (`-short`) skips those tests on its own. Do not add build
  tags or skip guards.
- A test that needs no container runs under `make test`, so it must be fast and deterministic: its result is cached.
- An integration test starts with `testutils.Parallel(t)`, **not** `t.Parallel()`.
  - With a license (`THEHIVE_TEST_LICENSE`), each test gets its own organisation and user, and the tests run in parallel.
  - Without one, all tests share one organisation. `Parallel` then does nothing, so the tests run one after another and each one purges its data before the next
    starts.
  - Pure unit tests keep `t.Parallel()`.
- Tests live in external packages (`package manage_test`), and each package's `TestMain` calls `testutils.TeardownContainers`.

## Helpers worth knowing (`internal/testutils/`)

- `CallToolOK` / `CallTool` and `StructuredData` call a tool and return its structured content.
- `RequirePermissionDenied`, `RequireScopeDenied` and `RequireConformsToOutputSchema` are the assertions for permissions, scope and schema.
- `GetMCPTestClientWithPermissions(t, testutils.PermissionsFixture(t, "analyst.yaml"))` builds a client with a given permission profile. Pass `""` for the
  default read-only profile.
- `MockInputCase`, `MockInputAlert`, `MockInputObservable`, … build fixtures to create through the raw thehive4go client
  (`hiveClient.CaseAPI.CreateCase(authContext)...`).
- Assertions use testify `require` and `assert`.

## Running

- `make test RUN=<regexp>` runs the unit tests that match. `make test-race` runs the unit tests with the race detector: use it after a concurrency change.
- `make test-integration RUN=<regexp> [THEHIVE_TEST_IMAGE=strangebee/thehive:5.5.16]` uses 5.8.0 unless you set the image. The stack stays up for fast reruns;
  `make test-integration-down` removes it.
- License modes are decided by `scripts/reset-integration-db.sh`, in this order: a `THEHIVE_TEST_LICENSE` you supply; otherwise one it mints from a pullable
  licensing image; otherwise the free license, which runs the tests one by one (slowest). The script prints the mode it picked.
- TheHive 5.5 and 5.6 do not always answer the same way. For example, a duplicate organisation gets 409 on 5.6 but 400 with "already exists" on 5.5 (see
  `organisationAlreadyExists`). When a test fails on one version only, check this before blaming the code.

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
- `MockInputCase`, `MockInputAlert`, `MockInputCaseTemplate` and `MockInputTask` build fixtures to create through the raw thehive4go client
  (`hiveClient.CaseAPI.CreateCase(authContext)...`).
- Assertions use testify `require` and `assert`.

## Running

- `make test RUN=<regexp>` runs the unit tests that match. `make test-race` runs the unit tests with the race detector: use it after a concurrency change.
- `make test-integration RUN=<regexp> [THEHIVE_TEST_IMAGE=strangebee/thehive:5.5.16]` uses 5.8.0 unless you set the image. The stack stays up for fast reruns;
  `make test-integration-down` removes it.
- License modes are decided by `bin/harness integration (internal/harness/stack)`, in this order: a `THEHIVE_TEST_LICENSE` you supply; otherwise one it mints
  from a pullable licensing image; otherwise the free license, which runs the tests one by one (slowest). It prints the mode it picked.
- TheHive 5.5 and 5.6 do not always answer the same way. For example, a duplicate organisation gets 409 on 5.6 but 400 with "already exists" on 5.5 (see
  `organisationAlreadyExists`). When a test fails on one version only, check this before blaming the code.

## What the pr gates hold the tests to

- **Integrity:** a changed test may not be removed, skip, or lose assertions or table cases (a helper given `t` counts as one), and a helper that takes `t` may
  not lose its own checks or gain a skip while it is kept. A test file may not leave CI's build (linux/amd64, no cgo) by its build constraint or its name
  (`_windows_test.go`, a leading `_`). A person who accepts it adds `path_test.go:TestName: why` (or `:build`) to `integrity.allow` in `gates/limits.yaml`.
- **Coverage:** each package keeps the unit-test coverage floor of `gates/coverage.yaml`. Test the code you add; raise a floor when its package gains tests.
- **Mutation (nightly):** a package with a floor in `gates/mutation.yaml` must kill that share of gremlins' mutants. The nightly holds every one to it;
  `bin/harness mutation` checks the packages you changed, before you push. A mutant that lives is a behaviour no test pins: the gate names the lines, and
  gremlins' report is in `.cache/harness/mutation/`.
- **Race and benchmarks:** `make test-race` must stay clean, and `bin/harness bench` holds the hot paths to `gates/bench.yaml` against the base.
- **Fuzzing:** `FuzzUntrustedWrap`, `FuzzFilterDSL`, `FuzzPermissionProfile` and `FuzzRetypeDateFields` run their seeds with the unit tests and fuzz nightly. An
  input the fuzzer finds lands in the package's `testdata/fuzz/`: fix the code and commit it as a seed.

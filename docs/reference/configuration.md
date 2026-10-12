# Configuration reference

This page lists every environment variable, command-line flag and HTTP header the server reads, their priority, and the built-in permission profiles.

## Configuration priority (highest to lowest)

1. **🌐 HTTP Request Headers** (HTTP transport only) - Per-request overrides for multi-tenant scenarios
2. **⚡ Command-line Flags** - Explicit runtime parameters
3. **🔧 Environment Variables** - System-level defaults (a `.env` file is loaded only from the server's working directory)

This allows you to set defaults via environment variables while overriding specific values per-request via headers or command-line flags.

## Configuration parameters

| Parameter                   | Environment variable            | Command-line flag                 | HTTP header                            | Default               | Description                                                                                                                    |
| --------------------------- | ------------------------------- | --------------------------------- | -------------------------------------- | --------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| **TheHive connection**      |
| TheHive URL                 | `THEHIVE_URL`                   | `--thehive-url`                   | `X-TheHive-Url`                        | -                     | TheHive instance URL (required)                                                                                                |
| API key                     | `THEHIVE_API_KEY`               | `--thehive-api-key`               | `Authorization` or `X-TheHive-Api-Key` | -                     | TheHive API key                                                                                                                |
| Username                    | `THEHIVE_USERNAME`              | `--thehive-username`              | -                                      | -                     | Username for basic auth                                                                                                        |
| Password                    | `THEHIVE_PASSWORD`              | `--thehive-password`              | -                                      | -                     | Password for basic auth                                                                                                        |
| Organisation                | `THEHIVE_ORGANISATION`          | `--thehive-organisation`          | `X-TheHive-Org`                        | -                     | TheHive organisation (optional, defaults to user's own)                                                                        |
| Request timeout             | `THEHIVE_REQUEST_TIMEOUT`       | `--thehive-request-timeout`       | -                                      | `2m`                  | Maximum wait for TheHive to answer one request; past it, the tool call fails with an explicit timeout error                    |
| **HTTP transport security** |
| URL allowlist               | `THEHIVE_URL_ALLOWLIST`         | `--thehive-url-allowlist`         | -                                      | `THEHIVE_URL` only    | Comma-separated TheHive base URLs clients may target via `X-TheHive-Url`; exact scheme/host/port match                         |
| Env credential fallback     | `ALLOW_ENV_CREDENTIAL_FALLBACK` | `--allow-env-credential-fallback` | -                                      | `false`               | Allow HTTP requests without credentials to use the server's `THEHIVE_API_KEY`/username/password (single-user deployments only) |
| Auth cache TTL              | `AUTH_VALIDATION_CACHE_TTL`     | `--auth-validation-cache-ttl`     | -                                      | `60s`                 | How long a successful credential validation is cached before re-checking with TheHive                                          |
| **Permissions**             |
| Permissions config          | `PERMISSIONS_CONFIG`            | `--permissions-config`            | -                                      | `read_only`           | Permissions: `read_only`, `admin`, or YAML file path                                                                           |
| **MCP server**              |
| Transport type              | -                               | `--transport`                     | -                                      | `http`                | Transport mode: `http` or `stdio`                                                                                              |
| Bind address                | `MCP_BIND_HOST` + `MCP_PORT`    | `--addr`                          | -                                      | required in HTTP mode | HTTP server bind address (for example, `0.0.0.0:8082`); `--addr` wins over the two variables, which must both be set otherwise |
| Endpoint path               | `MCP_SERVER_ENDPOINT`           | `--mcp-endpoint-path`             | -                                      | `/mcp`                | HTTP endpoint path                                                                                                             |
| Heartbeat interval          | `MCP_HEARTBEAT_INTERVAL`        | `--mcp-heartbeat-interval`        | -                                      | `30s`                 | Heartbeat interval for HTTP connections                                                                                        |
| **Cortex**                  |
| Default Cortex ID           | `CORTEX_ID`                     | `--cortex-id`                     | -                                      | `local`               | Default Cortex instance ID for analyzer/responder execution                                                                    |
| **Logging**                 |
| Log level                   | `LOG_LEVEL`                     | `--log-level`                     | -                                      | `info`                | Logging level                                                                                                                  |

## Per-request headers

`X-TheHive-Url` is only accepted when it matches the server's `THEHIVE_URL` or an entry in `THEHIVE_URL_ALLOWLIST` — any other destination is rejected before
TheHive is contacted. Requests must supply their own credentials unless `ALLOW_ENV_CREDENTIAL_FALLBACK=true`.

A `curl` example that sets headers per request is in [Remote Docker — HTTP deployment](../how-to/remote-docker.md#override-settings-per-request).

## Built-in permission profiles

- **`read_only`** (default, also used when `PERMISSIONS_CONFIG` is empty): Search entities, browse resources only
- **`admin`**: Allows all four tools, every entity operation and every analyzer and responder, with no filters (development/testing only). It is not the same as
  [permissions-examples/admin.yaml](permissions-examples/admin.yaml), a YAML profile you can adapt.
- **Custom file**: Fine-grained permissions via YAML configuration

See [permissions.md](permissions.md) for detailed permission configuration and examples.

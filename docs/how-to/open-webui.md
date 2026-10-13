# How to chat with TheHive from Open WebUI

This guide runs [Open WebUI](https://docs.openwebui.com/) with TheHiveMCP as its MCP tool server, so a web chat can search and read TheHive. It suits a local or
lab machine; for a shared deployment, put both behind a TLS-terminating, authenticating reverse proxy, as in [Remote Docker](remote-docker.md).

The stack is [`docker-compose.open-webui.yml`](../../deployment/examples/docker-compose.open-webui.yml): two containers, Open WebUI on `127.0.0.1:3000`, and
TheHiveMCP reachable from Open WebUI only. Open WebUI supports MCP natively over streamable HTTP since v0.6.31, so no proxy sits between them.

## Prerequisites

- Docker with Docker Compose
- TheHive 5.5+ and an API key for the user the chat should act as
- A key for an OpenAI-compatible endpoint, such as [OpenRouter](https://openrouter.ai/), OpenAI, or a local Ollama. Pick a model with reliable tool calling.

## 1. Set the variables

Write them to a `.env` file in the directory you run `docker compose` from (or export each one in your shell, and leave `--env-file .env` out of the next
command):

```bash
THEHIVE_URL=https://thehive.example.com   # as seen from a container: http://host.docker.internal:9000 for a TheHive on this machine
THEHIVE_API_KEY=<thehive_api_key>
THEHIVE_ORGANISATION=<organisation>       # optional, defaults to the key's own organisation
PERMISSIONS_CONFIG=read_only              # optional, the default; see the permissions reference before widening it
OPENAI_API_KEY=<llm_api_key>
OPENAI_API_BASE_URL=https://openrouter.ai/api/v1   # optional, the default
WEBUI_SECRET_KEY=<a long random string>   # for example: openssl rand -hex 32
```

## 2. Start the stack

```bash
docker compose -f deployment/examples/docker-compose.open-webui.yml --env-file .env up -d
```

Open <http://localhost:3000> and create the first account: Open WebUI makes it the administrator.

## 3. Check the TheHive tool server

On first start, `TOOL_SERVER_CONNECTIONS` in the compose file registers TheHiveMCP under **Admin Settings → Integrations → External Tool Servers** as an **MCP
(Streamable HTTP)** connection to `http://thehivemcp:8082/mcp`, with **Bearer** auth. Open WebUI sends that key as `Authorization: Bearer <key>`, and TheHiveMCP
uses it as the TheHive API key. Use the connection's verify button to check it answers.

Open WebUI stores this setting in its database after the first start, so later edits of the variable are ignored: change the connection in Admin Settings, or
remove the `open-webui` volume to start over.

The connection is visible to administrators only. To let other users call TheHive, give their groups access in the connection's settings. Every user then acts
with the same TheHive key: use a key whose rights fit all of them.

## 4. Ask a question

In a new chat, select a model, enable **TheHive** under **+ → Integrations → Tools**, and ask:

```text
Show me the 5 most recent high-severity alerts in TheHive.
```

The model calls `search-entities`, and Open WebUI shows each tool call under the answer.

## Troubleshooting

- **No tools are offered**: the tool server is not enabled in the chat, or the model does not support tool calling. In the model's settings, set **Function
  Calling** to **Native**.
- **The tool call fails with an authentication error**: the TheHive key is wrong, or `THEHIVE_URL` is not reachable from the container. Read
  `docker compose -f deployment/examples/docker-compose.open-webui.yml logs thehivemcp`.
- **"not in the allowlist"**: a client sent an `X-TheHive-Url` other than `THEHIVE_URL`. List the extra instances in `THEHIVE_URL_ALLOWLIST`, as explained in
  the [configuration reference](../reference/configuration.md).

## Next steps

- To let the chat create and update entities: [Configure permissions](configure-permissions.md)
- Every variable, flag and header: [configuration reference](../reference/configuration.md)

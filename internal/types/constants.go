package types

import "time"

type CtxKey string

const HiveAPIKeyCtxKey CtxKey = "hive_api_key"
const HiveOrgCtxKey CtxKey = "hive_org"
const HiveClientCtxKey CtxKey = "hive_client"
const PermissionsCtxKey CtxKey = "permissions"
const RequestIDCtxKey CtxKey = "request_id"
const OpenAIRequestStartTimeCtxKey CtxKey = "openai_request_start_time"
const SamplingModelRequestStartTimeCtxKey CtxKey = "sampling_model_request_start_time"

type EnvKey string

const EnvKeyTheHiveURL EnvKey = "THEHIVE_URL"
const EnvKeyTheHiveAPIKey EnvKey = "THEHIVE_API_KEY"
const EnvKeyTheHiveUsername EnvKey = "THEHIVE_USERNAME"
const EnvKeyTheHivePassword EnvKey = "THEHIVE_PASSWORD"
const EnvKeyTheHiveOrganisation EnvKey = "THEHIVE_ORGANISATION"
const EnvKeyPermissionsConfig EnvKey = "PERMISSIONS_CONFIG"
const EnvKeyMCPServerEndpoint EnvKey = "MCP_SERVER_ENDPOINT"
const EnvKeyMCPHeartbeatInterval EnvKey = "MCP_HEARTBEAT_INTERVAL"
const EnvKeyMCPPort EnvKey = "MCP_PORT"
const EnvKeyBindHost EnvKey = "MCP_BIND_HOST"
const EnvKeyLogLevel EnvKey = "LOG_LEVEL"
const EnvKeyOpenAIBaseURL EnvKey = "OPENAI_BASE_URL"
const EnvKeyOpenAIAPIKey EnvKey = "OPENAI_API_KEY"
const EnvKeyOpenAIModel EnvKey = "OPENAI_MODEL"
const EnvKeyOpenAIMaxTokens EnvKey = "OPENAI_MAX_TOKENS"

type FlagVar string

const FlagVarTheHiveURL FlagVar = "thehive-url"
const FlagVarTheHiveAPIKey FlagVar = "thehive-api-key"
const FlagVarTheHiveUsername FlagVar = "thehive-username"
const FlagVarTheHivePassword FlagVar = "thehive-password"
const FlagVarTheHiveOrganisation FlagVar = "thehive-organisation"
const FlagVarPermissionsConfig FlagVar = "permissions-config"
const FlagVarMCPServerEndpointPath FlagVar = "mcp-endpoint-path"
const FlagVarMCPHeartbeatInterval FlagVar = "mcp-heartbeat-interval"
const FlagVarTransportType FlagVar = "transport"
const FlagVarLogLevel FlagVar = "log-level"
const FlagVarOpenAIBaseURL FlagVar = "openai-base-url"
const FlagVarOpenAIAPIKey FlagVar = "openai-api-key"
const FlagVarOpenAIModel FlagVar = "openai-model"
const FlagVarOpenAIMaxTokens FlagVar = "openai-max-tokens"
const FlagVarBindAddr FlagVar = "addr"

type HeaderKey string

const HeaderKeyTheHiveAPIKey HeaderKey = "X-TheHive-Api-Key"
const HeaderKeyTheHiveOrganisation HeaderKey = "X-TheHive-Org"

type PermissionConfig string

const PermissionConfigReadOnly PermissionConfig = "read_only"
const PermissionConfigAdmin PermissionConfig = "admin"

const DefaultMaxCompletionTime = time.Duration(60 * 1000 * 1000 * 1000) // 60 seconds
const DefaultMaxCompletionRetries = 3

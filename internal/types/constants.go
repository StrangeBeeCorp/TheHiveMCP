package types

import (
	"time"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

type CtxKey string

const HiveAPIKeyCtxKey CtxKey = "hive_api_key"
const HiveOrgCtxKey CtxKey = "hive_org"
const HiveURLCtxKey CtxKey = "hive_url"
const HiveClientCtxKey CtxKey = "hive_client"
const PermissionsCtxKey CtxKey = "permissions"
const RequestIDCtxKey CtxKey = "request_id"
const OpenAIRequestStartTimeCtxKey CtxKey = "openai_request_start_time"
const SamplingModelRequestStartTimeCtxKey CtxKey = "sampling_model_request_start_time"
const OpenAIClientCtxKey CtxKey = "openai_client"
const OpenAIAPIKeyCtxKey CtxKey = "openai_api_key"
const OpenAIBaseURLCtxKey CtxKey = "openai_base_url"
const OpenAIModelCtxKey CtxKey = "openai_model"
const OpenAIMaxTokensCtxKey CtxKey = "openai_max_tokens"

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
const HeaderKeyTheHiveURL HeaderKey = "X-TheHive-Url"
const HeaderKeyOpenAIAPIKey HeaderKey = "X-OpenAI-Api-Key"
const HeaderKeyOpenAIBaseURL HeaderKey = "X-OpenAI-Base-Url"
const HeaderKeyOpenAIModelName HeaderKey = "X-OpenAI-Model-Name"
const HeaderKeyOpenAIMaxTokens HeaderKey = "X-OpenAI-Max-Tokens"

type PermissionConfig string

const PermissionConfigReadOnly PermissionConfig = "read_only"
const PermissionConfigAdmin PermissionConfig = "admin"

const DefaultMaxCompletionTime = 60 * time.Second
const DefaultMaxCompletionRetries = 3

// Entity type constants for TheHive entities
const (
	EntityTypeAlert      = "alert"
	EntityTypeCase       = "case"
	EntityTypeTask       = "task"
	EntityTypeObservable = "observable"
	EntityTypeComment    = "comment"
	EntityTypePage       = "page"
	EntityTypeAttachment = "attachment"
	EntityTypeTaskLog    = "task-log"
)

// OutputEntity is a union type representing possible output entities
type OutputEntity interface {
	thehive.OutputAlert |
		thehive.OutputCase |
		thehive.OutputTask |
		thehive.OutputObservable |
		map[string]interface{}
}

var DefaultFields map[string][]string = map[string][]string{
	EntityTypeAlert:      {"_id", "title", "_createdAt", "severity", "status"},
	EntityTypeCase:       {"_id", "title", "_createdAt", "status", "severity"},
	EntityTypeTask:       {"_id", "title", "status", "_createdAt", "assignee"},
	EntityTypeObservable: {"_id", "dataType", "_createdAt"},
	EntityTypeComment:    {"_id", "content", "_createdAt", "_createdBy"},
	EntityTypePage:       {"_id", "title", "_createdAt"},
	EntityTypeAttachment: {"_id", "fileName", "size", "_createdAt"},
	EntityTypeTaskLog:    {"_id", "message", "_createdAt", "_createdBy"},
}

const DateFormat = "2006-01-02T15:04:05"

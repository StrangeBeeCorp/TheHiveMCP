// Package types defines the configuration options, context/env/flag keys, and
// shared entity constants used across the TheHiveMCP server.
package types

import (
	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

// CtxKey is the type for keys stored in request contexts.
type CtxKey string

// HiveAPIKeyCtxKey is the context key holding the TheHive API key.
const HiveAPIKeyCtxKey CtxKey = "hive_api_key" // #nosec G101 -- context key name, not a real credential

// HiveOrgCtxKey is the context key holding the TheHive organisation.
const HiveOrgCtxKey CtxKey = "hive_org"

// HiveURLCtxKey is the context key holding the TheHive URL.
const HiveURLCtxKey CtxKey = "hive_url"

// HiveClientCtxKey is the context key holding the TheHive client.
const HiveClientCtxKey CtxKey = "hive_client"

// PermissionsCtxKey is the context key holding the resolved permissions.
const PermissionsCtxKey CtxKey = "permissions"

// RequestIDCtxKey is the context key holding the request ID.
const RequestIDCtxKey CtxKey = "request_id"

// AuthErrorCtxKey is the context key holding an authentication error.
const AuthErrorCtxKey CtxKey = "auth_error"

// AuthValidatedCtxKey is the context key marking a validated authentication.
const AuthValidatedCtxKey CtxKey = "auth_validated"

// DefaultCortexIDCtxKey is the context key holding the default Cortex ID.
const DefaultCortexIDCtxKey CtxKey = "default_cortex_id"

// EnvKey is the type for supported environment variable names.
type EnvKey string

// EnvKeyTheHiveURL is the env var name for TheHive URL.
const EnvKeyTheHiveURL EnvKey = "THEHIVE_URL"

// EnvKeyTheHiveAPIKey is the env var name for TheHive API key.
const EnvKeyTheHiveAPIKey EnvKey = "THEHIVE_API_KEY" // #nosec G101 -- env var name, not a real credential

// EnvKeyTheHiveUsername is the env var name for TheHive username.
const EnvKeyTheHiveUsername EnvKey = "THEHIVE_USERNAME"

// EnvKeyTheHivePassword is the env var name for TheHive password.
const EnvKeyTheHivePassword EnvKey = "THEHIVE_PASSWORD" // #nosec G101 -- env var name, not a real credential

// EnvKeyTheHiveOrganisation is the env var name for TheHive organisation.
const EnvKeyTheHiveOrganisation EnvKey = "THEHIVE_ORGANISATION"

// EnvKeyTheHiveURLAllowlist is the env var name for TheHive URL allowlist.
const EnvKeyTheHiveURLAllowlist EnvKey = "THEHIVE_URL_ALLOWLIST"

// EnvKeyAllowEnvCredentialFallback is the env var name for the env credential fallback toggle.
const EnvKeyAllowEnvCredentialFallback EnvKey = "ALLOW_ENV_CREDENTIAL_FALLBACK" // #nosec G101 -- env var name, not a real credential

// EnvKeyAuthValidationCacheTTL is the env var name for the auth validation cache TTL.
const EnvKeyAuthValidationCacheTTL EnvKey = "AUTH_VALIDATION_CACHE_TTL"

// EnvKeyPermissionsConfig is the env var name for the permissions config path.
const EnvKeyPermissionsConfig EnvKey = "PERMISSIONS_CONFIG"

// EnvKeyMCPServerEndpoint is the env var name for the MCP server endpoint path.
const EnvKeyMCPServerEndpoint EnvKey = "MCP_SERVER_ENDPOINT"

// EnvKeyMCPHeartbeatInterval is the env var name for the MCP heartbeat interval.
const EnvKeyMCPHeartbeatInterval EnvKey = "MCP_HEARTBEAT_INTERVAL"

// EnvKeyMCPPort is the env var name for the MCP server port.
const EnvKeyMCPPort EnvKey = "MCP_PORT"

// EnvKeyBindHost is the env var name for the MCP server bind host.
const EnvKeyBindHost EnvKey = "MCP_BIND_HOST"

// EnvKeyLogLevel is the env var name for the log level.
const EnvKeyLogLevel EnvKey = "LOG_LEVEL"

// EnvKeyCortexID is the env var name for the default Cortex ID.
const EnvKeyCortexID EnvKey = "CORTEX_ID"

// FlagVar is the type for supported CLI flag names.
type FlagVar string

// FlagVarTheHiveURL is the CLI flag name for TheHive URL.
const FlagVarTheHiveURL FlagVar = "thehive-url"

// FlagVarTheHiveAPIKey is the CLI flag name for TheHive API key.
const FlagVarTheHiveAPIKey FlagVar = "thehive-api-key" // #nosec G101 -- flag name, not a real credential

// FlagVarTheHiveUsername is the CLI flag name for TheHive username.
const FlagVarTheHiveUsername FlagVar = "thehive-username"

// FlagVarTheHivePassword is the CLI flag name for TheHive password.
const FlagVarTheHivePassword FlagVar = "thehive-password" // #nosec G101 -- flag name, not a real credential

// FlagVarTheHiveOrganisation is the CLI flag name for TheHive organisation.
const FlagVarTheHiveOrganisation FlagVar = "thehive-organisation"

// FlagVarTheHiveURLAllowlist is the CLI flag name for TheHive URL allowlist.
const FlagVarTheHiveURLAllowlist FlagVar = "thehive-url-allowlist"

// FlagVarAllowEnvCredentialFallback is the CLI flag name for the env credential fallback toggle.
const FlagVarAllowEnvCredentialFallback FlagVar = "allow-env-credential-fallback" // #nosec G101 -- flag name, not a real credential

// FlagVarAuthValidationCacheTTL is the CLI flag name for the auth validation cache TTL.
const FlagVarAuthValidationCacheTTL FlagVar = "auth-validation-cache-ttl"

// FlagVarPermissionsConfig is the CLI flag name for the permissions config path.
const FlagVarPermissionsConfig FlagVar = "permissions-config"

// FlagVarMCPServerEndpointPath is the CLI flag name for the MCP server endpoint path.
const FlagVarMCPServerEndpointPath FlagVar = "mcp-endpoint-path"

// FlagVarMCPHeartbeatInterval is the CLI flag name for the MCP heartbeat interval.
const FlagVarMCPHeartbeatInterval FlagVar = "mcp-heartbeat-interval"

// FlagVarTransportType is the CLI flag name for the transport type.
const FlagVarTransportType FlagVar = "transport"

// FlagVarLogLevel is the CLI flag name for the log level.
const FlagVarLogLevel FlagVar = "log-level"

// FlagVarBindAddr is the CLI flag name for the bind address.
const FlagVarBindAddr FlagVar = "addr"

// FlagVarCortexID is the CLI flag name for the default Cortex ID.
const FlagVarCortexID FlagVar = "cortex-id"

// HeaderKey is the type for supported HTTP header names.
type HeaderKey string

// HeaderKeyTheHiveAPIKey is the HTTP header carrying the TheHive API key.
const HeaderKeyTheHiveAPIKey HeaderKey = "X-TheHive-Api-Key" // #nosec G101 -- header name, not a real credential

// HeaderKeyTheHiveOrganisation is the HTTP header carrying the TheHive organisation.
const HeaderKeyTheHiveOrganisation HeaderKey = "X-TheHive-Org"

// HeaderKeyTheHiveURL is the HTTP header carrying the TheHive URL.
const HeaderKeyTheHiveURL HeaderKey = "X-TheHive-Url"

// PermissionConfig is the type for built-in permission configuration names.
type PermissionConfig string

// PermissionConfigReadOnly is the built-in read-only permission configuration.
const PermissionConfigReadOnly PermissionConfig = "read_only"

// PermissionConfigAdmin is the built-in admin permission configuration.
const PermissionConfigAdmin PermissionConfig = "admin"

// DefaultCortexID is the default Cortex instance ID used when none is specified.
const DefaultCortexID = "local"

// Field names shared across DefaultFields entries.
const (
	fieldID        = "_id"
	fieldCreatedAt = "_createdAt"
	fieldSeverity  = "severity"
	fieldStatus    = "status"
	fieldTitle     = "title"
)

// Entity type constants for TheHive entities
const (
	EntityTypeAlert        = "alert"
	EntityTypeCase         = "case"
	EntityTypeTask         = "task"
	EntityTypeObservable   = "observable"
	EntityTypeComment      = "comment"
	EntityTypePage         = "page"
	EntityTypeAttachment   = "attachment"
	EntityTypeTaskLog      = "task-log"
	EntityTypeProcedure    = "procedure"
	EntityTypePattern      = "pattern"
	EntityTypeCaseTemplate = "case-template"
)

// OutputEntity is a union type representing possible output entities
type OutputEntity interface {
	thehive.OutputAlert |
		thehive.OutputCase |
		thehive.OutputTask |
		thehive.OutputObservable |
		map[string]any
}

// DefaultFields maps each entity type to the default set of fields returned for it.
var DefaultFields = map[string][]string{
	EntityTypeAlert:        {fieldID, fieldTitle, fieldCreatedAt, fieldSeverity, fieldStatus},
	EntityTypeCase:         {fieldID, fieldTitle, fieldCreatedAt, fieldStatus, fieldSeverity},
	EntityTypeTask:         {fieldID, fieldTitle, fieldStatus, fieldCreatedAt, "assignee"},
	EntityTypeObservable:   {fieldID, "dataType", fieldCreatedAt},
	EntityTypeComment:      {fieldID, "message", fieldCreatedAt, "_createdBy"},
	EntityTypePage:         {fieldID, fieldTitle, fieldCreatedAt},
	EntityTypeAttachment:   {fieldID, "fileName", "size", fieldCreatedAt},
	EntityTypeTaskLog:      {fieldID, "message", fieldCreatedAt, "_createdBy"},
	EntityTypeProcedure:    {fieldID, "patternId", "patternName", "description", "occurDate"},
	EntityTypePattern:      {fieldID, "patternId", "name", "tactics", "platforms"},
	EntityTypeCaseTemplate: {fieldID, "name", "displayName", "description", fieldSeverity, "tags"},
}

// DateFormat is the timestamp layout used for TheHive date fields.
const DateFormat = "2006-01-02T15:04:05"

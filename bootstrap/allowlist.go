// Package bootstrap constructs the MCP server, registers its tools, resources,
// and prompts, and serves them over the configured transport (stdio or http),
// including authentication middleware, the TheHive URL allowlist, and the
// credential validation cache.
package bootstrap

import (
	"fmt"
	"net/url"
	"strings"
)

// TheHiveURLAllowlist holds the set of TheHive base URLs that HTTP clients are
// permitted to target via the X-TheHive-Url header. URLs are compared on
// normalized scheme, host, and port — exact host match only, no suffix or
// substring matching.
type TheHiveURLAllowlist struct {
	allowed map[string]struct{}
}

// normalizeTheHiveURL reduces a TheHive base URL to a canonical
// "scheme://host:port" form for exact comparison. Default ports are made
// explicit so "https://hive.example.com" and "https://hive.example.com:443"
// compare equal.
func normalizeTheHiveURL(rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", fmt.Errorf("invalid TheHive URL %q: %w", rawURL, err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("invalid TheHive URL %q: scheme must be http or https", rawURL)
	}

	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return "", fmt.Errorf("invalid TheHive URL %q: missing host", rawURL)
	}

	port := parsed.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}

	return fmt.Sprintf("%s://%s:%s", scheme, host, port), nil
}

// NewTheHiveURLAllowlist builds an allowlist from the configured entries plus
// the server's own TheHive URL. When no entries are configured, only the
// server's own URL is permitted; an empty allowlist with no server URL denies
// every request. Invalid entries are rejected at construction so
// misconfiguration fails at startup rather than falling open at request time.
func NewTheHiveURLAllowlist(entries []string, serverURL string) (*TheHiveURLAllowlist, error) {
	allowed := make(map[string]struct{}, len(entries)+1)

	for _, entry := range entries {
		normalized, err := normalizeTheHiveURL(entry)
		if err != nil {
			return nil, fmt.Errorf("invalid TheHive URL allowlist entry: %w", err)
		}

		allowed[normalized] = struct{}{}
	}

	// The server's own configured URL is always trusted
	if serverURL != "" {
		normalized, err := normalizeTheHiveURL(serverURL)
		if err != nil {
			return nil, fmt.Errorf("invalid TheHive URL: %w", err)
		}

		allowed[normalized] = struct{}{}
	}

	return &TheHiveURLAllowlist{allowed: allowed}, nil
}

// Allows reports whether the given TheHive URL exactly matches an allowlist
// entry. URLs that fail to parse or normalize are rejected.
func (a *TheHiveURLAllowlist) Allows(rawURL string) bool {
	if a == nil {
		return false
	}

	normalized, err := normalizeTheHiveURL(rawURL)
	if err != nil {
		return false
	}

	_, ok := a.allowed[normalized]

	return ok
}

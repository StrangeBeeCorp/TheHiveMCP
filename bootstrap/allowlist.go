package bootstrap

import (
	"fmt"
	"net/url"
	"strings"
)

// TheHiveURLAllowlist gates the TheHive base URLs clients may target via the
// X-TheHive-Url header. Match is exact on normalized scheme/host/port — no
// suffix or substring matching.
type TheHiveURLAllowlist struct {
	allowed map[string]struct{}
}

// normalizeTheHiveURL canonicalizes a URL to "scheme://host:port", making the
// default port explicit so "https://h.example" == "https://h.example:443".
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

// NewTheHiveURLAllowlist builds an allowlist from entries plus the server's own
// URL. With neither, every request is denied. Invalid entries are rejected here
// so misconfiguration fails at startup rather than falling open at request time.
func NewTheHiveURLAllowlist(entries []string, serverURL string) (*TheHiveURLAllowlist, error) {
	allowed := make(map[string]struct{}, len(entries)+1)

	for _, entry := range entries {
		normalized, err := normalizeTheHiveURL(entry)
		if err != nil {
			return nil, fmt.Errorf("invalid TheHive URL allowlist entry: %w", err)
		}
		allowed[normalized] = struct{}{}
	}

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

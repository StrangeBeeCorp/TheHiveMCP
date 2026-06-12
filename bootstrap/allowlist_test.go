package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTheHiveURLAllowlist_RejectsInvalidConfiguration(t *testing.T) {
	for name, args := range map[string][2]interface{}{
		"non-http allowlist entry": {[]string{"ftp://thehive.example.com"}, ""},
		"unparseable entry":        {[]string{"not a url"}, ""},
		"entry without host":       {[]string{"https://"}, ""},
		"non-http server URL":      {[]string(nil), "gopher://thehive.example.com"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewTheHiveURLAllowlist(args[0].([]string), args[1].(string))
			assert.Error(t, err)
		})
	}
}

func TestTheHiveURLAllowlist_Allows(t *testing.T) {
	cases := []struct {
		name      string
		entries   []string
		serverURL string
		url       string
		allowed   bool
	}{
		{name: "empty allowlist with no server URL denies everything", url: "https://thehive.example.com"},
		{name: "empty allowlist permits the server's own URL", serverURL: "https://thehive.example.com", url: "https://thehive.example.com", allowed: true},
		{name: "empty allowlist denies other hosts", serverURL: "https://thehive.example.com", url: "https://attacker.example.com"},
		{name: "exact entry match", entries: []string{"https://thehive.com", "http://hive.internal:9000"}, url: "https://thehive.com", allowed: true},
		{name: "exact entry match with explicit port", entries: []string{"http://hive.internal:9000"}, url: "http://hive.internal:9000", allowed: true},
		{name: "subdomain of an entry", entries: []string{"https://thehive.com"}, url: "https://other.thehive.com"},
		{name: "different port than entry", entries: []string{"http://hive.internal:9000"}, url: "http://hive.internal:9001"},
		{name: "host suffix bypass attempt", entries: []string{"https://thehive.com"}, url: "https://thehive.com.evil.test"},
		{name: "host prefix bypass attempt", entries: []string{"https://thehive.com"}, url: "https://evil-thehive.com"},
		{name: "allowed host in path", entries: []string{"https://thehive.com"}, url: "https://thehive.com.evil.test/thehive.com"},
		{name: "allowed host in query", entries: []string{"https://thehive.com"}, url: "https://evil.test/?u=thehive.com"},
		{name: "allowed host as userinfo", entries: []string{"https://thehive.com"}, url: "https://thehive.com@evil.test"},
		{name: "default https port is normalized", entries: []string{"https://thehive.com"}, url: "https://thehive.com:443", allowed: true},
		{name: "scheme and host are case-insensitive", entries: []string{"https://thehive.com"}, url: "HTTPS://THEHIVE.COM", allowed: true},
		{name: "trailing slash is ignored", entries: []string{"https://thehive.com"}, url: "https://thehive.com/", allowed: true},
		{name: "same host with different scheme", entries: []string{"https://thehive.com"}, url: "http://thehive.com"},
		{name: "same host with non-default port", entries: []string{"https://thehive.com"}, url: "https://thehive.com:8443"},
		{name: "empty URL", entries: []string{"https://thehive.com"}, url: ""},
		{name: "unparseable URL", entries: []string{"https://thehive.com"}, url: "not a url"},
		{name: "non-http scheme", entries: []string{"https://thehive.com"}, url: "ftp://thehive.com"},
		{name: "scheme-relative URL", entries: []string{"https://thehive.com"}, url: "//thehive.com"},
		{name: "explicit allowlist still trusts the server's own URL", entries: []string{"https://other.thehive.com"}, serverURL: "https://thehive.com", url: "https://thehive.com", allowed: true},
		{name: "explicit allowlist entries are honored alongside the server URL", entries: []string{"https://other.thehive.com"}, serverURL: "https://thehive.com", url: "https://other.thehive.com", allowed: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			allowlist, err := NewTheHiveURLAllowlist(tc.entries, tc.serverURL)
			require.NoError(t, err)
			assert.Equal(t, tc.allowed, allowlist.Allows(tc.url))
		})
	}

	t.Run("nil allowlist denies everything", func(t *testing.T) {
		var allowlist *TheHiveURLAllowlist
		assert.False(t, allowlist.Allows("https://thehive.com"))
	})
}

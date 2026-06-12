package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTheHiveURLAllowlist(t *testing.T) {
	t.Run("rejects invalid allowlist entries at construction", func(t *testing.T) {
		_, err := NewTheHiveURLAllowlist([]string{"ftp://thehive.example.com"}, "")
		assert.Error(t, err)

		_, err = NewTheHiveURLAllowlist([]string{"not a url"}, "")
		assert.Error(t, err)

		_, err = NewTheHiveURLAllowlist([]string{"https://"}, "")
		assert.Error(t, err)
	})

	t.Run("rejects invalid server URL at construction", func(t *testing.T) {
		_, err := NewTheHiveURLAllowlist(nil, "gopher://thehive.example.com")
		assert.Error(t, err)
	})

	t.Run("empty allowlist with no server URL denies everything", func(t *testing.T) {
		allowlist, err := NewTheHiveURLAllowlist(nil, "")
		require.NoError(t, err)
		assert.False(t, allowlist.Allows("https://thehive.example.com"))
	})
}

func TestTheHiveURLAllowlist_Allows(t *testing.T) {
	t.Run("empty allowlist permits only the server's own URL", func(t *testing.T) {
		allowlist, err := NewTheHiveURLAllowlist(nil, "https://thehive.example.com")
		require.NoError(t, err)

		assert.True(t, allowlist.Allows("https://thehive.example.com"))
		assert.False(t, allowlist.Allows("https://attacker.example.com"))
	})

	t.Run("matches exact entries only", func(t *testing.T) {
		allowlist, err := NewTheHiveURLAllowlist([]string{"https://thehive.com", "http://hive.internal:9000"}, "")
		require.NoError(t, err)

		assert.True(t, allowlist.Allows("https://thehive.com"))
		assert.True(t, allowlist.Allows("http://hive.internal:9000"))
		assert.False(t, allowlist.Allows("https://other.thehive.com"))
		assert.False(t, allowlist.Allows("http://hive.internal:9001"))
	})

	t.Run("no host suffix or substring bypass", func(t *testing.T) {
		allowlist, err := NewTheHiveURLAllowlist([]string{"https://thehive.com"}, "")
		require.NoError(t, err)

		assert.False(t, allowlist.Allows("https://thehive.com.evil.test"))
		assert.False(t, allowlist.Allows("https://evil-thehive.com"))
		assert.False(t, allowlist.Allows("https://thehive.com.evil.test/thehive.com"))
		assert.False(t, allowlist.Allows("https://evil.test/?u=thehive.com"))
		assert.False(t, allowlist.Allows("https://thehive.com@evil.test"))
	})

	t.Run("normalizes default ports and case", func(t *testing.T) {
		allowlist, err := NewTheHiveURLAllowlist([]string{"https://thehive.com"}, "")
		require.NoError(t, err)

		assert.True(t, allowlist.Allows("https://thehive.com:443"))
		assert.True(t, allowlist.Allows("HTTPS://THEHIVE.COM"))
		assert.True(t, allowlist.Allows("https://thehive.com/"))
		// Same host but different scheme or non-default port is a different endpoint
		assert.False(t, allowlist.Allows("http://thehive.com"))
		assert.False(t, allowlist.Allows("https://thehive.com:8443"))
	})

	t.Run("rejects unparseable or non-http URLs", func(t *testing.T) {
		allowlist, err := NewTheHiveURLAllowlist([]string{"https://thehive.com"}, "")
		require.NoError(t, err)

		assert.False(t, allowlist.Allows(""))
		assert.False(t, allowlist.Allows("not a url"))
		assert.False(t, allowlist.Allows("ftp://thehive.com"))
		assert.False(t, allowlist.Allows("//thehive.com"))
	})

	t.Run("nil allowlist denies everything", func(t *testing.T) {
		var allowlist *TheHiveURLAllowlist
		assert.False(t, allowlist.Allows("https://thehive.com"))
	})

	t.Run("explicit allowlist still trusts the server's own URL", func(t *testing.T) {
		allowlist, err := NewTheHiveURLAllowlist([]string{"https://other.thehive.com"}, "https://thehive.com")
		require.NoError(t, err)

		assert.True(t, allowlist.Allows("https://other.thehive.com"))
		assert.True(t, allowlist.Allows("https://thehive.com"))
	})
}

package bootstrap

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

// DefaultAuthValidationCacheTTL is how long a successful credential validation
// is cached when no TTL is configured.
const DefaultAuthValidationCacheTTL = 60 * time.Second

// validationCache remembers recent successful TheHive credential validations
// so the HTTP transport does not round-trip to TheHive on every request. Only
// successes are cached; failed validations are always retried upstream.
type validationCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	expires map[[sha256.Size]byte]time.Time
}

func newValidationCache(ttl time.Duration) *validationCache {
	if ttl <= 0 {
		ttl = DefaultAuthValidationCacheTTL
	}
	return &validationCache{
		ttl:     ttl,
		expires: make(map[[sha256.Size]byte]time.Time),
	}
}

// key derives the cache key from the credentials. The raw API key is hashed so
// it is not kept in memory longer than necessary.
func (c *validationCache) key(creds *TheHiveCredentials) [sha256.Size]byte {
	return sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s", creds.URL, creds.APIKey, creds.Organisation)))
}

// IsValid reports whether the credentials were successfully validated within
// the TTL window. Expired entries are evicted on access.
func (c *validationCache) IsValid(creds *TheHiveCredentials) bool {
	key := c.key(creds)

	c.mu.Lock()
	defer c.mu.Unlock()

	expiry, ok := c.expires[key]
	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		delete(c.expires, key)
		return false
	}
	return true
}

// MarkValid records a successful validation for the credentials.
func (c *validationCache) MarkValid(creds *TheHiveCredentials) {
	key := c.key(creds)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.expires[key] = time.Now().Add(c.ttl)
}

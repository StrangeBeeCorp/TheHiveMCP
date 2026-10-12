package bootstrap

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// cacheCreds are credentials with key, for the cache's tests.
func cacheCreds(key string) *TheHiveCredentials {
	return &TheHiveCredentials{URL: "http://thehive", APIKey: key, Organisation: "org"}
}

// The cache is shared by every request of the HTTP transport: marking and checking credentials
// from many goroutines at once is safe, with no request in between to order them (the race gate
// runs this under -race), and each credential is valid once marked.
func TestTheValidationCacheIsSafeForConcurrentUse(t *testing.T) {
	t.Parallel()

	cache := newValidationCache(time.Minute)
	key := func(i int) *TheHiveCredentials { return cacheCreds(fmt.Sprintf("key-%d", i%8)) }

	var wg sync.WaitGroup

	for i := range 64 {
		wg.Go(func() {
			cache.MarkValid(key(i))
			_ = cache.IsValid(key(i + 1))
		})
	}

	wg.Wait()

	for i := range 8 {
		assert.True(t, cache.IsValid(key(i)), "key-%d was marked", i)
	}

	assert.False(t, cache.IsValid(cacheCreds("never")))
}

// An entry expires after its TTL, and is evicted when checked.
func TestAValidationExpiresAfterItsTTL(t *testing.T) {
	t.Parallel()

	cache := newValidationCache(time.Nanosecond)
	c := cacheCreds("key")

	cache.MarkValid(c)
	time.Sleep(time.Millisecond)

	assert.False(t, cache.IsValid(c))
	assert.Empty(t, cache.expires, "the expired entry is evicted")
}

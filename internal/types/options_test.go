package types

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTheHiveRequestTimeout(t *testing.T) {
	t.Parallel()

	valid := map[string]time.Duration{
		"":    DefaultTheHiveRequestTimeout,
		"90s": 90 * time.Second,
		"5m":  5 * time.Minute,
	}
	for value, want := range valid {
		got, err := ParseTheHiveRequestTimeout(value)
		require.NoError(t, err, value)
		assert.Equal(t, want, got, value)
	}

	for _, value := range []string{"0s", "-1m", "two minutes", "120"} {
		_, err := ParseTheHiveRequestTimeout(value)
		assert.Error(t, err, value)
	}
}

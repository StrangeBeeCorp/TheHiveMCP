package utils

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/analyzers"
)

// The untrusted analyzer trusts exactly the fields the wrapper lets through unmarked: its trusted
// fields and its date fields. A field added to one list and not the other fails here.
func TestTheAnalyzerTrustsWhatTheWrapperTrusts(t *testing.T) {
	t.Parallel()

	var wrapper []string
	for name := range trustedFields {
		wrapper = append(wrapper, name)
	}

	wrapper = append(wrapper, dateFields...)
	slices.Sort(wrapper)

	assert.Equal(t, slices.Compact(wrapper), analyzers.TrustedFields())
}

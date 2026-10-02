package checks_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
)

const changelogBefore = `# Changelog

## [Unreleased]

### Fixed

- A filter on an unindexed field fails with a hint.

## [1.0.0] - 2026-09-01

- The first release.
`

// An entry counts when it is new under ## [Unreleased]: not a heading, not a blank line, not an
// edit of an older release.
func TestAnEntryCountsOnlyUnderUnreleased(t *testing.T) {
	t.Parallel()

	for name, after := range map[string]string{
		"a new entry":                  changelogBefore[:len("# Changelog\n\n## [Unreleased]\n\n")] + "- A tool says what was cut.\n" + changelogBefore[len("# Changelog\n\n## [Unreleased]\n\n"):],
		"a new entry in a new section": "# Changelog\n\n## [Unreleased]\n\n### Changed\n\n- search-entities says what it cut.\n",
	} {
		assert.True(t, checks.UnreleasedAdded(changelogBefore, after), name)
	}

	for name, after := range map[string]string{
		"nothing changed":       changelogBefore,
		"an older release only": changelogBefore + "- A fix to an old entry.\n",
		"a heading only":        "# Changelog\n\n## [Unreleased]\n\n### Fixed\n\n### Added\n\n- A filter on an unindexed field fails with a hint.\n",
		"no changelog at all":   "",
	} {
		assert.False(t, checks.UnreleasedAdded(changelogBefore, after), name)
	}

	assert.True(t, checks.UnreleasedAdded("", "## [Unreleased]\n\n- The first entry.\n"), "a changelog the base did not have")
}

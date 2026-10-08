package gatesdoc_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gatesdoc"
)

func TestTablePadsColumnsLikePrettier(t *testing.T) {
	t.Parallel()

	got := gatesdoc.Table([]string{"Gate", "Blocks"}, [][]string{{"`lint`", "a finding"}, {"`structure`", "a | pipe"}})
	want := "| Gate        | Blocks    |\n" +
		"| ----------- | --------- |\n" +
		"| `lint`      | a finding |\n" +
		"| `structure` | a \\| pipe |\n"
	assert.Equal(t, want, got)
}

func TestWrapFillsLinesWithoutBreakingCode(t *testing.T) {
	t.Parallel()

	text := strings.Repeat("word ", 40) + "`make gate ID=<gate>` " + strings.Repeat("tail ", 20)
	for line := range strings.SplitSeq(gatesdoc.Wrap(text), "\n") {
		assert.LessOrEqual(t, utf8.RuneCountInString(line), 160)
		assert.Equal(t, 0, strings.Count(line, "`")%2, "a code span is split across lines: %q", line)
	}
}

// A link's text is kept whole on one line, as prettier keeps it: the generated page then stays as
// prettier leaves it.
func TestWrapKeepsALinksTextWhole(t *testing.T) {
	t.Parallel()

	text := strings.Repeat("word ", 30) + "see [Work with the harness](../how-to/work-with-the-harness.md) and " + strings.Repeat("more ", 30)

	for line := range strings.SplitSeq(gatesdoc.Wrap(text), "\n") {
		assert.Equal(t, strings.Count(line, "["), strings.Count(line, "]"), "a link's text split across lines: %q", line)
	}
}

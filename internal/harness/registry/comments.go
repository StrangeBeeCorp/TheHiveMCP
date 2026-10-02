package registry

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// TrailingComments refuses a comment on the line of a value. In an unquoted text, " #" starts a
// comment, so a rule that names a Markdown heading ("## [Unreleased]") would stop there without a
// word. The harness's YAML files keep their comments on lines of their own.
func TrailingComments(data []byte) error {
	var doc yaml.Node

	err := yaml.Unmarshal(data, &doc)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	var lines []string

	var walk func(n *yaml.Node)

	walk = func(n *yaml.Node) {
		if n.LineComment != "" {
			lines = append(lines, strconv.Itoa(n.Line))
		}

		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(&doc)

	// A key and its value share a line, and either may carry the comment.
	lines = slices.Compact(lines)
	if len(lines) == 0 {
		return nil
	}

	return fmt.Errorf("a comment after a value on line %s: in an unquoted text, \" #\" starts a comment and cuts the "+
		"text there; quote the text, fold it with >-, or put the comment on a line of its own", strings.Join(lines, ", "))
}

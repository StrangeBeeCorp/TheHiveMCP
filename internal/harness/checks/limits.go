// Package checks holds the gates built into the harness: structure, public-safety and docs.
// Each check reads the repository and returns findings; a finding blocks.
package checks

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/registry"
)

// Finding is one problem a check found. Line is 0 when it concerns the whole file.
type Finding struct {
	Path string
	Line int
	Msg  string
}

func (f Finding) String() string {
	if f.Line > 0 {
		return f.Path + ":" + strconv.Itoa(f.Line) + ": " + f.Msg
	}

	if f.Path != "" {
		return f.Path + ": " + f.Msg
	}

	return f.Msg
}

// Limits is gates/limits.yaml: what the structure and public-safety gates enforce.
type Limits struct {
	Structure    StructureLimits `yaml:"structure"`
	PublicSafety SafetyRules     `yaml:"public_safety"`
	Hooks        HookLimits      `yaml:"hooks"`
}

// HookLimits bound what the Claude Code hooks let an agent do.
type HookLimits struct {
	// MaxRedRounds is how many times an agent may stop red before it must write BLOCKED.md.
	MaxRedRounds int `yaml:"max_red_rounds"`
}

// StructureLimits bounds file and directory size.
type StructureLimits struct {
	MaxLines           int            `yaml:"max_lines"`
	MaxCodeFilesPerDir int            `yaml:"max_code_files_per_dir"`
	CodeFiles          []string       `yaml:"code_files"`
	Caps               map[string]int `yaml:"caps"`
	Root               []string       `yaml:"root"`
}

// SafetyRules says what must never be committed to this public repository.
type SafetyRules struct {
	ForbiddenPaths []string `yaml:"forbidden_paths"`
	AllowedPaths   []string `yaml:"allowed_paths"`
	HomeNames      []string `yaml:"home_names"`
	SkipContent    []string `yaml:"skip_content"`
}

// LoadLimits reads gates/limits.yaml, rejecting unknown keys.
func LoadLimits(file string) (Limits, error) {
	data, err := os.ReadFile(file) // #nosec G304 -- the path is the repository's own limits file
	if err != nil {
		return Limits{}, fmt.Errorf("read limits: %w", err)
	}

	return ParseLimits(data, file)
}

// ParseLimits decodes limits, rejecting unknown keys; file names them in errors.
func ParseLimits(data []byte, file string) (Limits, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var l Limits

	err := dec.Decode(&l)
	if err != nil {
		return Limits{}, fmt.Errorf("parse %s: %w", file, err)
	}

	err = registry.TrailingComments(data)
	if err != nil {
		return Limits{}, fmt.Errorf("%s: %w", file, err)
	}

	if l.Hooks.MaxRedRounds <= 0 {
		return Limits{}, fmt.Errorf("%s: hooks.max_red_rounds must be at least 1", file)
	}

	return l, nil
}

// matchesFile matches a pattern against the full path when it has a slash, else the base name:
// "*.go" means any Go file, ".githooks/*" the files of that directory.
func matchesFile(patterns []string, name string) bool {
	base := path.Base(name)
	for _, p := range patterns {
		if registry.Match(p, name) || (!containsSlash(p) && registry.Match(p, base)) {
			return true
		}
	}

	return false
}

func containsSlash(s string) bool {
	return strings.Contains(s, "/")
}

// sniffLen is how much of a file decides whether it is binary, as git does.
const sniffLen = 8000

// isBinary reports whether the content looks binary: a NUL byte in its first 8000 bytes.
func isBinary(content []byte) bool {
	return bytes.IndexByte(content[:min(len(content), sniffLen)], 0) >= 0
}

// countLines counts lines, a last line without a newline included.
func countLines(content []byte) int {
	n := bytes.Count(content, []byte{'\n'})
	if len(content) > 0 && content[len(content)-1] != '\n' {
		n++
	}

	return n
}

func validText(content []byte) bool {
	return utf8.Valid(content)
}

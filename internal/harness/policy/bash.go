package policy

import (
	"slices"
	"strings"
	"unicode"
)

// Bash decides whether a command line may run: unless it names an environment file, it may. What
// the command does is Claude Code's to judge, and what it changed is the Stop hook's (see the
// package comment).
func Bash(line string) Decision {
	if slices.ContainsFunc(strings.FieldsFunc(line, wordBreak), IsEnvFile) {
		return deny(envSecrets)
	}

	return Allow
}

// wordBreak splits a command line into the words that may name a file: on blanks, quotes, and the
// characters that join a path to an option or to another command (= : ; | & < > ( ) and `).
func wordBreak(r rune) bool {
	return unicode.IsSpace(r) || strings.ContainsRune("\"'=:;|&<>()`", r)
}

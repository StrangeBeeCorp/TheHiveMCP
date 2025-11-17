package version

import (
	"fmt"
	"strings"
)

var (
	gitVersion = "dev"     // injected via -ldflags
	gitCommit  = "unknown" // injected via -ldflags
	buildDate  = "unknown" // injected via -ldflags
)

// Info returns a human readable single-line version.
func Info() string {
	return fmt.Sprintf("%s (%s) %s", gitVersion, gitCommit, buildDate)
}

// GitVersion returns the semantic version (tag) or pre-release id.
func GitVersion() string { return gitVersion }

// GitCommit returns the git commit hash.
func GitCommit() string { return gitCommit }

// BuildDate returns the build date.
func BuildDate() string { return buildDate }

// GetVersion returns the version without the 'v' prefix for use in manifests and packages.
func GetVersion() string {
	return strings.TrimPrefix(gitVersion, "v")
}

// GetFullVersion returns the complete version with 'v' prefix as used in git tags.
func GetFullVersion() string {
	if !strings.HasPrefix(gitVersion, "v") {
		return "v" + gitVersion
	}
	return gitVersion
}

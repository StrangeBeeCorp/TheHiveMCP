package checks_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const onTag = "startsWith(github.ref, 'refs/tags/')"

// A manual run of the release workflow from a branch publishes its snapshot image alone: :latest
// moves, and a GitHub release is created, only for a tag.
func TestOnlyATagMovesLatestOrCutsARelease(t *testing.T) {
	t.Parallel()

	var wf struct {
		Jobs map[string]struct {
			If    string `yaml:"if"`
			Steps []struct {
				Uses string            `yaml:"uses"`
				With map[string]string `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}

	require.NoError(t, yaml.Unmarshal(repoFile(t, ".github/workflows/release.yml"), &wf))

	release, ok := wf.Jobs["release"]
	require.True(t, ok, "the release job")
	assert.Contains(t, release.If, onTag, "the release job runs for a tag only")

	latest := 0

	for name, job := range wf.Jobs {
		for _, step := range job.Steps {
			for line := range strings.SplitSeq(step.With["tags"], "\n") {
				if !strings.Contains(line, "latest") {
					continue
				}

				latest++

				assert.Contains(t, line, onTag, "job %s: :latest is pushed for a tag only", name)
			}
		}
	}

	assert.Equal(t, 1, latest, "one step pushes :latest")
}

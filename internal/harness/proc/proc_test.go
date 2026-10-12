package proc_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/proc"
)

// A command that runs and exits non-zero is no error: its code is its result, with what it
// printed, apart. Only a command that cannot start is an error.
func TestLocalReportsTheExitCodeAndTheOutput(t *testing.T) {
	t.Parallel()

	out, err := proc.Local(t.Context(), []string{"sh", "-c", "read line; echo out $line; echo err >&2; exit 3"}, strings.NewReader("in\n"))
	require.NoError(t, err)
	assert.Equal(t, proc.Output{Stdout: []byte("out in\n"), Stderr: []byte("err\n"), Code: 3}, out)
	assert.Equal(t, "out in\nerr\n", out.Text())

	_, err = proc.Local(t.Context(), []string{"/no/such/tool"}, nil)
	require.ErrorContains(t, err, "/no/such/tool")
}

func TestStreamedReportsTheExitCode(t *testing.T) {
	t.Parallel()

	out, err := proc.Streamed(t.Context(), []string{"sh", "-c", "exit 4"}, nil)
	require.NoError(t, err)
	assert.Equal(t, 4, out.Code)

	_, err = proc.Streamed(t.Context(), []string{"/no/such/tool"}, nil)
	require.Error(t, err)
}

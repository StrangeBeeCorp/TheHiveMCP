// Package proc runs the commands the harness drives (docker, the signing tool) through one type,
// so that the packages that build them are tested with a fake.
package proc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// Exec runs one command line with stdin. Its error is for a command that could not start.
type Exec func(ctx context.Context, argv []string, stdin io.Reader) (Output, error)

// Output is what a command printed, and its exit code.
type Output struct {
	Stdout []byte
	Stderr []byte
	Code   int
}

// Text is stdout then stderr, as a terminal shows them.
func (o Output) Text() string {
	return string(o.Stdout) + string(o.Stderr)
}

// Local runs the command on this machine and captures its output.
func Local(ctx context.Context, argv []string, stdin io.Reader) (Output, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) // #nosec G204 -- command lines the harness builds from its own pinned parts
	cmd.Stdin = stdin

	var stdout, stderr bytes.Buffer

	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()

	return result(argv[0], Output{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, err)
}

// Streamed runs the command on this machine with its output on this process's own: for a long
// run whose progress a person watches, such as the integration suite. Nothing is captured.
func Streamed(ctx context.Context, argv []string, stdin io.Reader) (Output, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) // #nosec G204 -- command lines the harness builds from its own pinned parts
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, os.Stdout, os.Stderr

	return result(argv[0], Output{}, cmd.Run())
}

func result(name string, out Output, err error) (Output, error) {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		out.Code = exit.ExitCode()
		return out, nil
	}

	if err != nil {
		return out, fmt.Errorf("%s: %w", name, err)
	}

	return out, nil
}

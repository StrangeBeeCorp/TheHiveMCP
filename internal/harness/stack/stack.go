package stack

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/proc"
)

// Stack is the integration stack of one checkout.
type Stack struct {
	Root string
	// Compose is the base compose file; LicenseOverride is layered on it in a licensed mode.
	Compose, LicenseOverride string
	// LicenseFile is where the token goes, relative to Root: the override mounts it, and an
	// empty file boots TheHive unlicensed in --dev mode, for the mint.
	LicenseFile string
	// TheHiveImage is THEHIVE_TEST_IMAGE; SuppliedLicense is THEHIVE_TEST_LICENSE.
	TheHiveImage, SuppliedLicense string
	// URL is TheHive's on the host; DevUser and DevPassword are its --dev bootstrap admin.
	URL, DevUser, DevPassword string
	// Run captures a command's output; Stream shows it as it runs.
	Run, Stream proc.Exec
	// Ready reports whether TheHive answers its status page; Wait bounds the wait for it, and
	// Poll spaces the tries.
	Ready      func(ctx context.Context) bool
	Wait, Poll time.Duration
	Log        io.Writer
}

// ErrNotReady is TheHive not answering in time.
var ErrNotReady = errors.New("TheHive did not answer")

// Prepare brings the stack up with its license and resets its databases, and returns the mode and
// the license token ("" in the free mode). On a failure the stack is torn down, so that no half
// started or half reset stack is left; on success it stays up for the next run, until make
// test-integration-down.
func (s Stack) Prepare(ctx context.Context) (Mode, string, error) {
	license := filepath.Join(s.Root, filepath.FromSlash(s.LicenseFile))

	// A token from a previous run must never leak into this one.
	err := os.Remove(license)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Free, "", fmt.Errorf("remove the old license: %w", err)
	}

	mode, image := Free, ""

	switch {
	case s.SuppliedLicense != "":
		mode = Supplied
		_, _ = fmt.Fprintln(s.Log, "THEHIVE_TEST_LICENSE is set → license (parallel) mode; no mint.")
	default:
		image = licensingImage(ctx, s.Run, s.TheHiveImage, s.Log)
		if image != "" {
			mode = Minted
			_, _ = fmt.Fprintf(s.Log, "Licensing image %s is pullable → license (parallel) mode.\n", image)
		} else {
			_, _ = fmt.Fprintln(s.Log, "Licensing image not pullable → free (sequential) mode.")
		}
	}

	if mode != Free {
		err = writeLicense(license, s.SuppliedLicense) // empty while it is minted
		if err != nil {
			return mode, "", err
		}
	}

	token, err := s.up(ctx, mode, image, license)
	if err != nil {
		_, _ = fmt.Fprintf(s.Log, "the stack could not be prepared (%v): tearing it down\n", err)
		_, _ = s.Stream(context.WithoutCancel(ctx), s.compose(mode, "down", "-v"), nil)

		return mode, "", err
	}

	_, _ = fmt.Fprintln(s.Log, "Stack is up and reset; ready for the test run.")

	return mode, token, nil
}

// up starts the stack, mints the license when it must, and resets the databases.
func (s Stack) up(ctx context.Context, mode Mode, image, license string) (string, error) {
	err := s.step(ctx, "start the stack", s.compose(mode, "up", "-d"))
	if err != nil {
		return "", err
	}

	token := s.SuppliedLicense

	if mode == Minted {
		token, err = s.mint(ctx, image)
		if err == nil {
			err = writeLicense(license, token)
		}

		if err != nil {
			return "", err
		}

		_, _ = fmt.Fprintln(s.Log, "License minted and activated.")
	}

	_, _ = fmt.Fprintln(s.Log, "Resetting databases (DROP keyspace, delete the Elasticsearch indices, restart TheHive)…")

	// IF EXISTS: no keyspace on a first run; a missing index is no error either. TheHive
	// re-creates its schema only at startup, hence the restart, which also re-activates the
	// license on disk against the new database. The suite waits for TheHive itself.
	for _, step := range []struct {
		what string
		args []string
	}{
		{"drop the keyspace", []string{"exec", "-T", "cassandra", "cqlsh", "-e", "DROP KEYSPACE IF EXISTS thehive;"}},
		{"delete the indices", []string{"exec", "-T", "elasticsearch", "sh", "-c", `curl -s -o /dev/null -X DELETE "http://localhost:9200/thehive*"`}}, // the dev stack's Elasticsearch serves http only, inside its container
		{"restart TheHive", []string{"restart", "thehive"}},
	} {
		err = s.step(ctx, step.what, s.compose(mode, step.args...))
		if err != nil {
			return "", err
		}
	}

	return token, nil
}

// mint waits for the --dev TheHive to answer, then mints its license.
func (s Stack) mint(ctx context.Context, image string) (string, error) {
	_, _ = fmt.Fprintln(s.Log, "Waiting for TheHive to accept the licensing challenge…")

	err := s.waitReady(ctx)
	if err != nil {
		return "", err
	}

	_, _ = fmt.Fprintln(s.Log, "Minting a multi-instance dev license…")

	argv, err := MintArgv(image, s.URL, s.DevUser, s.DevPassword)
	if err != nil {
		return "", err
	}

	out, err := s.Run(ctx, argv, nil)
	if err != nil {
		return "", fmt.Errorf("mint the license: %w", err)
	}

	token := Token(out.Text())
	if token == "" {
		return "", fmt.Errorf("mint the license: no token (exit %d); the minter printed:\n%s", out.Code, strings.TrimSpace(out.Text()))
	}

	return token, nil
}

func (s Stack) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(s.Wait)

	for {
		if s.Ready(ctx) {
			return nil
		}

		if !time.Now().Before(deadline) {
			return fmt.Errorf("%w at %s within %s", ErrNotReady, s.URL, s.Wait)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for TheHive: %w", ctx.Err())
		case <-time.After(s.Poll):
		}
	}
}

func (s Stack) step(ctx context.Context, what string, argv []string) error {
	out, err := s.Stream(ctx, argv, nil)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}

	if out.Code != 0 {
		return fmt.Errorf("%s: exit %d", what, out.Code)
	}

	return nil
}

// compose is a docker compose command on the base file, with the license override in a licensed
// mode.
func (s Stack) compose(mode Mode, args ...string) []string {
	argv := []string{dockerCmd, "compose", "-f", s.Compose}
	if mode != Free {
		argv = append(argv, "-f", s.LicenseOverride)
	}

	return append(argv, args...)
}

// licensePerm is world-readable: TheHive's container user reads the file through a read-only
// mount, whoever owns it on the host. It holds a dev license, and git ignores it.
const (
	licensePerm = 0o644
	dirPerm     = 0o750
)

func writeLicense(path, token string) error {
	err := os.MkdirAll(filepath.Dir(path), dirPerm)
	if err == nil {
		err = os.WriteFile(path, []byte(token), licensePerm) // #nosec G306 -- read by the container's user; a gitignored dev license
	}

	if err == nil {
		err = os.Chmod(path, licensePerm) // WriteFile's mode is masked by the umask
	}

	if err != nil {
		return fmt.Errorf("write the license file: %w", err)
	}

	return nil
}

// Tests is a go test run of the whole module against the stack.
type Tests struct {
	// Mount is the checkout's bind mount; Image the Go image, which has git.
	Mount, Image string
	// User, Caches and Tools are the Makefile's docker flags: the host user in CI, the module
	// and build caches, the installed tools' cache.
	User, Caches, Tools []string
	// Cover and Run are the go test flags of COVERAGE=1 and RUN=<regexp>.
	Cover, Run []string
	// Shared, when set, is the shared mode (see Shared): the suite runs in that organisation of that
	// TheHive, one package at a time, whatever the license.
	Shared *Target
	// Pass names the environment variables handed to the container by name, when the harness has
	// them set: the own-stack superadmin overrides of the Go suite.
	Pass []string
}

// gotestsum is installed once in the tools cache.
const gotestsum = "gotest.tools/gotestsum@v1.13.0"

// Argv runs the suite on the host network, where TheHive answers. With a license it runs in
// parallel, one organisation per test; without, one package at a time on the one shared
// organisation. The timeout is above go test's default: under memory pressure a request can stall,
// and the setup helpers retry it.
func (t Tests) Argv(license string) []string {
	flags := append(append([]string{}, t.Cover...), t.Run...)
	if license == "" || t.Shared != nil {
		flags = append(flags, "-p", "1")
	}

	flags = append(flags, "-timeout", "20m", "./...")

	quoted := make([]string, len(flags))
	for i, f := range flags {
		quoted[i] = shellQuote(f)
	}

	script := "command -v gotestsum >/dev/null 2>&1 || go install " + gotestsum +
		" ; gotestsum --format pkgname --hide-summary=skipped -- " + strings.Join(quoted, " ")

	argv := []string{dockerCmd, "run", "-i", "--rm", "--network", "host", "-v", t.Mount, "-w", "/app"}
	argv = append(argv, t.env(license)...)
	argv = append(append(append(argv, t.User...), t.Caches...), t.Tools...)

	return append(argv, t.Image, "sh", "-c", script)
}

// env is the container's environment. In the shared mode it also carries the target, by value:
// the harness holds it, not the ambient environment (the organisation-wide key of a lease, like
// the license above, is a short-lived dev credential).
func (t Tests) env(license string) []string {
	url := "THEHIVE_TEST_URL"
	if t.Shared != nil {
		url += "=" + t.Shared.URL
		license = "" // never parallel: one organisation serves every test
	}

	env := []string{
		"-e", url, "-e", "LOG_LEVEL", "-e", "THEHIVE_TEST_IMAGE", "-e", "HARNESS_LOAD", "-e", "THEHIVE_TEST_LICENSE=" + license, "-e", "CGO_ENABLED=0",
	}

	if t.Shared != nil {
		env = append(env, "-e", "THEHIVE_TEST_SHARED=1", "-e", "THEHIVE_TEST_ORG="+t.Shared.Org, "-e", "THEHIVE_TEST_API_KEY="+t.Shared.Key)
	}

	for _, name := range t.Pass {
		env = append(env, "-e", name)
	}

	return env
}

// shellQuote quotes a word for sh: a RUN regexp such as 'TestA|TestB' must reach go test whole.
func shellQuote(word string) string {
	if word != "" && strings.Trim(word, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_./=,:") == "" {
		return word
	}

	return "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
}

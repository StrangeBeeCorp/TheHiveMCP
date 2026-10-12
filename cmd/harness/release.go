package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/proc"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/release"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/stack"
)

// The release and integration commands.
const (
	cmdPackage     = "package"
	cmdSign        = "sign-windows"
	cmdIntegration = "integration"
)

// streamExec runs the long commands whose output a person or a CI log follows; the tests replace
// it, and toolExec, the captured one.
var streamExec proc.Exec = proc.Streamed

// packageCmd packs the built binaries: bin/harness package tarballs|mcpb [--version V] [--build
// DIR] [--dist DIR] [--name N] [--targets "linux-amd64 ..." | --host].
func (e env) packageCmd(ctx context.Context, args []string, out io.Writer) error {
	usage := usageError{`usage: harness package tarballs|mcpb [--version V] [--build DIR] [--dist DIR] [--name N] [--targets "T ..." | --host]`}
	if len(args) == 0 || (args[0] != "tarballs" && args[0] != "mcpb") {
		return usage
	}

	fs := flag.NewFlagSet(cmdPackage, flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	o := release.Options{}
	targets := fs.String("targets", "", "the GOOS-GOARCH targets, space-separated")
	host := fs.Bool("host", false, "the one target of this machine (mcpb: with PERMISSIONS_CONFIG, from the environment or the checkout's dotenv file)")
	fs.StringVar(&o.Version, "version", "v0.0.0", "the version in the package names")
	fs.StringVar(&o.Build, "build", "build", "the directory of the built binaries")
	fs.StringVar(&o.Dist, "dist", "dist", "the directory the packages go to")
	fs.StringVar(&o.Name, "name", "thehivemcp", "the binary's base name")

	if fs.Parse(args[1:]) != nil || fs.NArg() > 0 {
		return usage
	}

	o.Targets = strings.Fields(*targets)
	if *host {
		o.Targets = []string{runtime.GOOS + "-" + runtime.GOARCH}
	}

	o.DistAs = o.Dist

	for _, d := range []*string{&o.Build, &o.Dist} {
		if !filepath.IsAbs(*d) {
			*d = filepath.Join(e.root, *d)
		}
	}

	written, err := e.pack(ctx, args[0], o, *host)
	for _, w := range written {
		_, _ = fmt.Fprintf(out, "%s/%s (+ .sha256)\n", o.DistAs, w)
	}

	if err != nil {
		return fmt.Errorf("package: %w", err)
	}

	return nil
}

func (e env) pack(ctx context.Context, kind string, o release.Options, host bool) ([]string, error) {
	if kind == "tarballs" {
		return release.Tarballs(o) //nolint:wrapcheck // packageCmd wraps it
	}

	// A release's bundles start read-only: only this machine's bundle, a developer's, takes
	// PERMISSIONS_CONFIG, from the environment or, as the server reads it, the dotenv file.
	perms := release.Permissions{}

	if host {
		value := os.Getenv("PERMISSIONS_CONFIG")
		if value == "" {
			local, err := godotenv.Read(filepath.Join(e.root, ".env"))
			if err == nil {
				value = local["PERMISSIONS_CONFIG"]
			}
		}

		perms = release.PermissionsFrom(value, e.root)
	}

	return release.MCPBs(ctx, streamExec, o, filepath.Join(e.root, "docs", "images", "theHivelogo.png"), perms, //nolint:wrapcheck // packageCmd wraps it
		os.Getuid(), os.Getgid())
}

// signWindows signs the Windows binaries in place: bin/harness sign-windows BINARY...
func (e env) signWindows(ctx context.Context, args []string, out io.Writer) error {
	home, _ := os.UserHomeDir()

	return release.Signer{ //nolint:wrapcheck // its errors name the step
		Provider: os.Getenv("SIGNING_PROVIDER"), Env: os.Getenv, Look: exec.LookPath, Home: home, Run: streamExec, Out: out,
	}.Sign(ctx, args)
}

// The integration stack's files, and how long a --dev TheHive may take to answer before its
// license is minted.
const (
	composeFile     = "docker-compose.test.yml"
	licenseOverride = "docker-compose.license.yml"
	licenseFile     = "internal/testutils/testdata/.test-license.lic.local"
	theHiveURL      = "http://localhost:9000" // the dev stack's TheHive serves http only, on this machine
	readyWait       = 6 * time.Minute
	readyPoll       = 5 * time.Second
)

// lookPath finds devstack on the PATH for the shared mode; the tests replace it.
var lookPath = exec.LookPath

// readyCheck asks TheHive's status page; the tests replace it.
var readyCheck = func(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, readyPoll)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, theHiveURL+"/api/status", nil)
	if err != nil {
		return false
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}

	_ = resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// integration prepares the stack and runs the suite against it: bin/harness integration, as
// make test-integration calls it with the Go image and the docker flags in the environment.
// With THEHIVE_TEST_SHARED (make test-integration-shared) it runs the suite in one organisation of
// an already-running TheHive instead: no stack, no reset, no license, and the lease released at the
// end, whatever the suite did.
func (e env) integration(ctx context.Context, args []string, out io.Writer) error {
	if len(args) > 0 {
		return usageError{"usage: harness integration (RUN, COVERAGE and THEHIVE_TEST_IMAGE come from make test-integration, THEHIVE_TEST_SHARED from make test-integration-shared)"}
	}

	image := os.Getenv("GO_IMAGE")
	if image == "" {
		return errors.New("integration: GO_IMAGE is not set: run make test-integration")
	}

	t := stack.Tests{
		Mount: cmp.Or(os.Getenv("MOUNT_APP"), e.root+":/app"), Image: image,
		User: strings.Fields(os.Getenv("GO_RUN_AS_HOST_UID")), Caches: strings.Fields(os.Getenv("DOCKER_CACHE_MOUNTS")),
		Tools: strings.Fields(os.Getenv("GO_TOOLS_CACHE")), Cover: strings.Fields(os.Getenv("GO_TEST_COVER")), Run: strings.Fields(os.Getenv("GO_TEST_RUN")),
		Pass: passed("THEHIVE_TEST_ADMIN_USER", "THEHIVE_TEST_ADMIN_PASSWORD"),
	}

	license := ""

	if stack.WantsShared(os.Getenv("THEHIVE_TEST_SHARED")) {
		target, release, err := sharedTarget(ctx, out)
		defer release()

		if err != nil {
			return fmt.Errorf("integration: %w", err)
		}

		t.Shared = &target

		_, _ = fmt.Fprintf(out, "Mode: shared (sequential): organisation %s at %s; no reset.\n", target.Org, target.URL)
	} else {
		var err error

		license, err = e.prepare(ctx, out)
		if err != nil {
			return fmt.Errorf("integration: %w", err)
		}
	}

	res, err := streamExec(ctx, t.Argv(license), nil)
	if err != nil {
		return fmt.Errorf("integration: run the suite: %w", err)
	}

	if res.Code != 0 {
		return errFailed
	}

	return nil
}

// passed keeps the variables of names that the environment sets.
func passed(names ...string) []string {
	var set []string

	for _, name := range names {
		if _, ok := os.LookupEnv(name); ok {
			set = append(set, name)
		}
	}

	return set
}

// prepare brings the compose stack up with its license and resets it, and returns the license
// token ("" in the free mode): the default mode of the suite.
func (e env) prepare(ctx context.Context, out io.Writer) (string, error) {
	s := stack.Stack{
		Root: e.root, Compose: composeFile, LicenseOverride: licenseOverride, LicenseFile: licenseFile,
		TheHiveImage: os.Getenv("THEHIVE_TEST_IMAGE"), SuppliedLicense: os.Getenv("THEHIVE_TEST_LICENSE"),
		URL: theHiveURL, DevUser: cmp.Or(os.Getenv("THEHIVE_DEV_USER"), "admin"), DevPassword: cmp.Or(os.Getenv("THEHIVE_DEV_PASSWORD"), "secret"),
		Run: toolExec, Stream: streamExec, Ready: readyCheck, Wait: readyWait, Poll: readyPoll, Log: out,
	}

	mode, license, err := s.Prepare(ctx)
	if err != nil {
		return "", err //nolint:wrapcheck // Prepare's errors name the step
	}

	_, _ = fmt.Fprintf(out, "Mode: %s.\n", mode)

	return license, nil
}

// sharedTarget finds the organisation of an already-running TheHive that the shared mode runs in
// (make test-integration-shared), from THEHIVE_TEST_URL, _ORG, _API_KEY and _API_KEY_FILE, or by
// leasing one from devstack. The returned function releases the lease; call it whatever happens.
func sharedTarget(ctx context.Context, out io.Writer) (stack.Target, func(), error) {
	return stack.Shared{ //nolint:wrapcheck // Acquire's errors name the step
		URL: os.Getenv("THEHIVE_TEST_URL"), Org: os.Getenv("THEHIVE_TEST_ORG"),
		Key: os.Getenv("THEHIVE_TEST_API_KEY"), KeyFile: os.Getenv("THEHIVE_TEST_API_KEY_FILE"),
		Run: toolExec, Look: lookPath, ReadFile: os.ReadFile, Log: out,
	}.Acquire(ctx)
}

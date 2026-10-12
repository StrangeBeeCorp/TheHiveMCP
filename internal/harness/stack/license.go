// Package stack runs the integration suite against a live TheHive: it brings the stack of
// docker-compose.test.yml up with its license, resets its databases, and runs go test against it.
// bin/harness integration runs it for make test-integration and the integration gate.
//
// Why reset rather than recreate: a cold boot is mostly Cassandra (about a minute) and TheHive's
// schema bootstrap, so the stack stays up between runs and each run wipes its data instead: drop
// the JanusGraph keyspace, delete TheHive's Elasticsearch indices, restart TheHive. The reset
// must be complete: the per-test org and user counters (internal/testutils/orgs.go) restart from
// 0 in every test process, so a dirty database would hand the next run orgs that still hold data.
package stack

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/proc"
)

// Mode is how TheHive is licensed, which decides how the suite runs.
type Mode int

// The modes, by precedence:
//   - Supplied: THEHIVE_TEST_LICENSE holds a token (a CI secret, a local export), used as it is;
//   - Minted: the licensing image is pullable (an authenticated developer): TheHive boots in
//     --dev mode and a multi-instance dev license is minted for the run;
//   - Free: neither (the public mirror, external contributors): TheHive's built-in license, one
//     organisation, so the suite runs one package at a time.
//
// Supplied and Minted run the suite in parallel, one organisation per test.
const (
	Free Mode = iota
	Supplied
	Minted
)

func (m Mode) String() string {
	return [...]string{"free (sequential)", "license (parallel), supplied", "license (parallel), minted"}[m]
}

// dockerCmd runs every container of the stack.
const dockerCmd = "docker"

// The licensing image, and the tag tried when the TheHive image names no version: a license
// minted by any recent tag validates against 5.5, 5.6 and 5.7.
const (
	licensingRepo        = "ghcr.io/strangebee/tools/licensing"
	licensingFallbackTag = "release-5.6"
)

var majorMinor = regexp.MustCompile(`^(\d+)\.(\d+)`)

// LicensingTag is the licensing tag of a TheHive image: strangebee/thehive:5.6.3 is
// release-5.6; "" when the image names no version.
func LicensingTag(image string) string {
	i := strings.LastIndex(image, ":")
	if i < 0 {
		return ""
	}

	m := majorMinor.FindStringSubmatch(image[i+1:])
	if m == nil {
		return ""
	}

	return "release-" + m[1] + "." + m[2]
}

// licensingImage is the first licensing image docker can pull, the TheHive image's tag first,
// or "" for none. Each probe is logged, so a fall back to the free mode is never silent.
func licensingImage(ctx context.Context, run proc.Exec, theHiveImage string, log io.Writer) string {
	for _, tag := range []string{LicensingTag(theHiveImage), licensingFallbackTag} {
		if tag == "" {
			continue
		}

		image := licensingRepo + ":" + tag
		_, _ = fmt.Fprintf(log, "Probing %s…\n", image)

		out, err := run(ctx, []string{dockerCmd, "manifest", "inspect", image}, nil)
		if err == nil && out.Code == 0 {
			_, _ = fmt.Fprintln(log, "  → pullable")
			return image
		}

		why := strings.TrimSpace(string(out.Stderr))
		if err != nil {
			why = err.Error()
		}

		_, _ = fmt.Fprintf(log, "  → not pullable: %s\n", why)
	}

	return ""
}

// MintArgv mints a multi-instance dev license against the TheHive at theHiveURL, signed in as
// TheHive's --dev bootstrap admin, and prints it.
func MintArgv(image, theHiveURL, user, password string) ([]string, error) {
	u, err := url.Parse(theHiveURL)
	if err != nil {
		return nil, fmt.Errorf("the TheHive URL %q: %w", theHiveURL, err)
	}

	u.User = url.UserPassword(user, password)

	return []string{
		dockerCmd, "run", "--network", "host", "--rm", "-i", image,
		"--key-id", "dev", "--thehive", u.String(), "--expiration", "365days", "--customer", "StrangeBee",
		"--plan", "Platinum", "--multiInstance", "--quotas", "users.normal=-1,organisations=-1", "--stdout",
	}, nil
}

// Token is the license the minter printed: its line "Token: <jwt>", or "".
func Token(minted string) string {
	for line := range strings.SplitSeq(minted, "\n") {
		if t, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "Token: "); ok {
			return strings.TrimSpace(t)
		}
	}

	return ""
}

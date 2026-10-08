package dashboard

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/issues"
)

const (
	// Path is where make dashboard writes the page, in the checkout's cache.
	Path = ".cache/dashboard/index.html"
	// defaultWeeks is how far back the page looks.
	defaultWeeks = 26
	// httpTimeout bounds a call to Jira.
	httpTimeout = 30 * time.Second
)

// ErrUsage is a command line the dashboard cannot run.
var ErrUsage = errors.New("usage: harness dashboard [--weeks N] [--out path] [--open]")

// ghCLI runs gh in the checkout.
type ghCLI struct{ dir string }

func (g ghCLI) Run(ctx context.Context, args ...string) ([]byte, error) {
	out, err := issues.GH{Ctx: ctx, Dir: g.dir}.Run(nil, args...)
	if err != nil {
		return nil, fmt.Errorf("gh: %w", err)
	}

	return out, nil
}

// Command builds the page of the checkout at root (make dashboard), prints where it is and what
// it could not read, and with --open opens it in the browser.
func Command(ctx context.Context, root string, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("dashboard", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	weeks := fs.Int("weeks", defaultWeeks, "how many weeks back the page looks")
	dest := fs.String("out", Path, "where to write the page, from the repository's root")
	open := fs.Bool("open", false, "open the page in the browser")

	err := fs.Parse(args)
	if err != nil || *weeks < 1 || fs.NArg() > 0 {
		return ErrUsage
	}

	now := time.Now()

	main := mainBranch
	if gitx.HasRef(ctx, root, "origin/"+mainBranch) {
		main = "origin/" + mainBranch
	}

	var gh GitHub

	_, err = exec.LookPath("gh")
	if err == nil {
		gh = ghCLI{dir: root}
	}

	d := Collect(ctx, Options{
		Root: root, Main: main, Since: now.AddDate(0, 0, -daysAWeek**weeks), Now: now,
		GitHub: gh, Getenv: os.Getenv, HTTP: &http.Client{Timeout: httpTimeout},
	})

	p := filepath.Join(root, *dest)

	err = write(p, Build(d))
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(out, "dashboard: %s (%.1f s)\n", p, d.Built.Seconds())

	for _, source := range slices.Sorted(maps.Keys(d.Missing)) {
		_, _ = fmt.Fprintf(out, "  not read: %s: %s\n", source, d.Missing[source])
	}

	if *open {
		return openPage(ctx, p)
	}

	return nil
}

func write(p string, page Page) error {
	err := os.MkdirAll(filepath.Dir(p), 0o750) //nolint:mnd // the owner's and the group's
	if err != nil {
		return fmt.Errorf("dashboard: %w", err)
	}

	f, err := os.Create(p) // #nosec G304 -- under the checkout, where the person asked
	if err != nil {
		return fmt.Errorf("dashboard: %w", err)
	}

	err = Render(f, page)
	closeErr := f.Close()

	if err == nil && closeErr != nil {
		err = fmt.Errorf("dashboard: %w", closeErr)
	}

	return err
}

// openPage opens a file in the browser: open on macOS, xdg-open elsewhere.
func openPage(ctx context.Context, p string) error {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}

	err := exec.CommandContext(ctx, opener, p).Run() // #nosec G204 -- the system's opener on the page just written
	if err != nil {
		return fmt.Errorf("dashboard: open the page with %s: %w", opener, err)
	}

	return nil
}

package lint

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/proc"
)

// Runner runs steps in a checkout.
type Runner struct {
	Root string
	Exec proc.Exec
	// Watch are the files whose content a fix may change, relative to Root: a fix is reported by
	// the files it changed.
	Watch []string
}

// Failure is a step's finding: its name, and what the tool printed.
type Failure struct {
	Name   string
	Output string
}

// Report is what a run did: the files each kind of fix changed, and the checks that failed.
type Report struct {
	Fixed    map[Fix][]string
	Failures []Failure
}

const (
	dirPerm = 0o750
	// fragmentPerm is world-readable: checkmake's image runs as nobody, and ignores a
	// configuration it cannot read.
	fragmentPerm = 0o644
)

// Run pulls the images the steps need, then runs the steps in order.
func (r Runner) Run(ctx context.Context, steps []Step) Report {
	rep := Report{Fixed: map[Fix][]string{}}

	r.pull(ctx, steps)

	for _, s := range steps {
		if s.Fix != Check {
			before := r.hashes()

			if f, failed := r.fix(ctx, s); failed {
				rep.Failures = append(rep.Failures, f)
			}

			rep.Fixed[s.Fix] = append(rep.Fixed[s.Fix], changed(before, r.hashes())...)

			continue
		}

		if f, failed := r.check(ctx, s); failed {
			rep.Failures = append(rep.Failures, f)
		}
	}

	for k, files := range rep.Fixed {
		slices.Sort(files)
		rep.Fixed[k] = slices.Compact(files)
	}

	return rep
}

// pull fetches, quietly, each image the steps need that docker does not have yet: docker run
// would print its pull progress among the tool's findings.
func (r Runner) pull(ctx context.Context, steps []Step) {
	var images []string

	for _, s := range steps {
		if !slices.Contains(images, s.Image) {
			images = append(images, s.Image)
		}
	}

	for _, im := range images {
		out, err := r.Exec(ctx, []string{dockerCmd, "image", "inspect", "--format", "{{.Id}}", im}, nil)
		if err == nil && out.Code == 0 {
			continue
		}

		// A failed pull is left to docker run, whose error then says why.
		_, _ = r.Exec(ctx, []string{dockerCmd, "pull", "-q", im}, nil)
	}
}

// cannotRun is the first exit code of docker's own errors: 125 when docker run fails, 126 and
// 127 when the command cannot run in the container.
const cannotRun = 125

// fix runs one fix. What a tool could not fix is not a failure: lint's checks after it report
// it, and the fmt gate checks the files. A fix that could not run at all is one, and so is one
// killed before it finished (a negative code: a signal, or a cancelled context).
func (r Runner) fix(ctx context.Context, s Step) (Failure, bool) {
	out, err := r.execOut(ctx, s)
	if err != nil {
		return Failure{Name: s.Name, Output: err.Error()}, true
	}

	if out.Code >= cannotRun || out.Code < 0 {
		return Failure{Name: s.Name, Output: strings.TrimRight(string(out.Stdout)+string(out.Stderr), "\n")}, true
	}

	return Failure{}, false
}

// check runs one check and reads its result: a failure is a non-zero exit, or, from a tool that
// lists the files it would rewrite, any file on its stdout (its stderr is diagnostics, such as
// npm's notices).
func (r Runner) check(ctx context.Context, s Step) (Failure, bool) {
	out, err := r.execOut(ctx, s)
	if err != nil {
		return Failure{Name: s.Name, Output: err.Error()}, true
	}

	text := string(out.Stdout) + string(out.Stderr)

	if s.Lists {
		listed := strings.TrimSpace(strings.ReplaceAll(string(out.Stdout), app+"/", ""))
		if out.Code == 0 && listed == "" {
			return Failure{}, false
		}

		if out.Code == 0 || listed != "" {
			text = listed
		}
	}

	if out.Code != 0 || s.Lists {
		return Failure{Name: s.Name, Output: strings.TrimRight(text, "\n")}, true
	}

	return Failure{}, false
}

func (r Runner) execOut(ctx context.Context, s Step) (proc.Output, error) {
	if s.Before != nil {
		err := r.write(*s.Before)
		if err != nil {
			return proc.Output{}, err
		}
	}

	var stdin io.Reader

	if s.Stdin != "" {
		data, err := os.ReadFile(filepath.Join(r.Root, filepath.FromSlash(s.Stdin))) // #nosec G304 -- a file the run selected in the checkout
		if err != nil {
			return proc.Output{}, fmt.Errorf("read %s: %w", s.Stdin, err)
		}

		stdin = bytes.NewReader(data)
	}

	out, err := r.Exec(ctx, s.Argv, stdin)
	if err != nil {
		return proc.Output{}, fmt.Errorf("%s could not run: %w", s.Name, err)
	}

	return out, nil
}

func (r Runner) write(f File) error {
	p := filepath.Join(r.Root, filepath.FromSlash(f.Path))

	err := os.MkdirAll(filepath.Dir(p), dirPerm)
	if err != nil {
		return fmt.Errorf("write %s: %w", f.Path, err)
	}

	err = os.WriteFile(p, f.Content, fragmentPerm) // #nosec G306 -- read by the tool's container user; holds no secret
	if err == nil {
		err = os.Chmod(p, fragmentPerm) // WriteFile's mode is masked by the umask
	}

	if err != nil {
		return fmt.Errorf("write %s: %w", f.Path, err)
	}

	return nil
}

// hashes is each watched file's content hash; a file that cannot be read has none.
func (r Runner) hashes() map[string][sha256.Size]byte {
	h := make(map[string][sha256.Size]byte, len(r.Watch))

	for _, p := range r.Watch {
		data, err := os.ReadFile(filepath.Join(r.Root, filepath.FromSlash(p))) // #nosec G304 -- a file of the checkout
		if err == nil {
			h[p] = sha256.Sum256(data)
		}
	}

	return h
}

// changed are the files whose hash differs between before and after, sorted.
func changed(before, after map[string][sha256.Size]byte) []string {
	var out []string

	for p, sum := range after {
		if old, ok := before[p]; !ok || old != sum {
			out = append(out, p)
		}
	}

	slices.Sort(out)

	return out
}

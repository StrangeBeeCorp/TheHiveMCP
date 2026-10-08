// Package dashboard builds the repository's health page (make dashboard): who and which models
// contribute, how long the tests take, how often the repository merges and releases, how long a
// feature takes, and how its quality moves. Every number comes from a source read when the page is
// built: git history and make pr's notes, GitHub through gh, the tree, this clone's last make pr,
// and Jira when its credentials are set. A source that cannot be read is named on
// the page, in the sections it feeds; it never stops the build.
package dashboard

import (
	"context"
	"sync"
	"time"
)

// The sources, as the page names them.
const (
	SourceGit    = "git history"
	SourceNotes  = "make pr's notes"
	SourceGitHub = "GitHub (gh)"
	SourceTree   = "the tree"
	SourceLocal  = "this clone's last make pr"
	SourceJira   = "Jira"
)

// sourceTimeout bounds each source: the page is built in under a minute, whatever answers late.
const sourceTimeout = 40 * time.Second

// Options say where the page reads from.
type Options struct {
	// Root is the checkout.
	Root string
	// Main is the ref of the main branch, origin/main when there is one.
	Main string
	// Since is how far back the page looks.
	Since time.Time
	// Now is when the page is built.
	Now time.Time
	// GitHub runs gh; nil when it is not there.
	GitHub GitHub
	// Getenv reads the environment (Jira's credentials).
	Getenv func(string) string
	// HTTP calls Jira; nil leaves it out.
	HTTP HTTPDoer
}

// Data is everything the page is drawn from, with what each source could not give.
type Data struct {
	Repo    string
	Head    string
	Since   time.Time
	Now     time.Time
	Built   time.Duration
	Commits []Commit
	Merges  []time.Time
	Tags    []Tag
	Notes   []NoteRun
	GitHub  GitHubData
	Tree    TreeData
	Local   LocalRun
	Tickets []Ticket
	// Missing says, per source, why it could not be read.
	Missing map[string]string
}

// Collect reads every source at once, each within sourceTimeout.
func Collect(ctx context.Context, o Options) Data {
	start := time.Now()
	d := Data{Since: o.Since, Now: o.Now, Missing: map[string]string{}}

	var mu sync.Mutex

	miss := func(source string, err error) {
		if err == nil {
			return
		}

		mu.Lock()
		defer mu.Unlock()

		d.Missing[source] = err.Error()
	}

	sources := []func(context.Context){
		func(ctx context.Context) {
			h, err := readGit(ctx, o)
			d.Repo, d.Head, d.Commits, d.Merges, d.Tags = h.Repo, h.Head, h.Commits, h.Merges, h.Tags

			miss(SourceGit, err)
		},
		func(ctx context.Context) {
			var err error

			d.Notes, err = readNotes(ctx, o.Root)
			miss(SourceNotes, err)
		},
		func(ctx context.Context) {
			var errs map[string]error

			d.GitHub, errs = readGitHub(ctx, o.GitHub)
			for source, err := range errs {
				miss(source, err)
			}
		},
		func(ctx context.Context) {
			var err error

			d.Tree, err = readTree(ctx, o.Root)
			miss(SourceTree, err)
		},
		func(context.Context) {
			var err error

			d.Local, err = readLocal(o.Root)
			miss(SourceLocal, err)
		},
		func(ctx context.Context) {
			var err error

			d.Tickets, err = readJira(ctx, o)
			miss(SourceJira, err)
		},
	}

	var wg sync.WaitGroup

	for _, read := range sources {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(ctx, sourceTimeout)
			defer cancel()

			read(ctx)
		})
	}

	wg.Wait()

	d.Built = time.Since(start)

	return d
}

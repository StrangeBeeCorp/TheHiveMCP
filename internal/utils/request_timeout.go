package utils

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// TheHiveTimeoutError reports a request to TheHive that got no complete answer
// within the configured limit, so the server stopped waiting for it.
type TheHiveTimeoutError struct {
	Method  string
	Path    string
	Timeout time.Duration
	Err     error
}

func (e *TheHiveTimeoutError) Error() string {
	return fmt.Sprintf("TheHive did not answer %s %s within %s, so the request was abandoned", e.Method, e.Path, e.Timeout)
}

func (e *TheHiveTimeoutError) Unwrap() error {
	return e.Err
}

// Summary is the one-line statement a tool error leads with.
func (e *TheHiveTimeoutError) Summary() string {
	return fmt.Sprintf("TheHive did not answer within %s", e.Timeout)
}

// Hints tells the caller what the timeout means and what is safe to do next.
// The second one matters most: an abandoned write may still land in TheHive,
// and a blind retry is how duplicates get created.
func (e *TheHiveTimeoutError) Hints() []string {
	return []string{
		"TheHive is slow or unreachable; check that it is up. Retrying a read is safe.",
		"If this call created, updated or deleted something, TheHive may still have applied it after the server stopped waiting: read the entity back before retrying, or you may create a duplicate.",
		fmt.Sprintf("The limit is set by %s (currently %s).", types.EnvKeyTheHiveRequestTimeout, e.Timeout),
	}
}

// AsTheHiveTimeout returns the TheHive timeout carried anywhere in err's chain.
func AsTheHiveTimeout(err error) (*TheHiveTimeoutError, bool) {
	var timeoutErr *TheHiveTimeoutError
	if errors.As(err, &timeoutErr) {
		return timeoutErr, true
	}

	return nil, false
}

// TimeoutTransport bounds every request, response body included, to Timeout,
// and reports an expiry as a *TheHiveTimeoutError.
//
// It is a transport rather than http.Client.Timeout so the error can carry the
// limit and the request that hit it, which is what makes the tool error useful.
type TimeoutTransport struct {
	Transport http.RoundTripper
	Timeout   time.Duration
}

// RoundTrip implements http.RoundTripper.
func (t *TimeoutTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	next := t.Transport
	if next == nil {
		next = http.DefaultTransport
	}

	if t.Timeout <= 0 {
		return next.RoundTrip(req) //nolint:wrapcheck // pass-through when no limit is set
	}

	parent := req.Context()
	ctx, cancel := context.WithTimeout(parent, t.Timeout)

	resp, err := next.RoundTrip(req.WithContext(ctx))
	if err != nil {
		err = t.classify(ctx, parent, req, err)

		cancel()

		return nil, err
	}

	resp.Body = &timeoutBody{ReadCloser: resp.Body, transport: t, req: req, ctx: ctx, parent: parent, cancel: cancel}

	return resp, nil
}

// classify turns err into a *TheHiveTimeoutError when our own deadline caused
// it. A caller that cancelled or set a shorter deadline is not our timeout.
func (t *TimeoutTransport) classify(ctx, parent context.Context, req *http.Request, err error) error {
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) || parent.Err() != nil {
		return err
	}

	return &TheHiveTimeoutError{Method: req.Method, Path: req.URL.Path, Timeout: t.Timeout, Err: err}
}

// timeoutBody keeps the deadline alive while the body is read, and releases it
// on Close.
type timeoutBody struct {
	io.ReadCloser

	transport *TimeoutTransport
	req       *http.Request
	ctx       context.Context //nolint:containedctx // the deadline must outlive RoundTrip until the body is read
	parent    context.Context //nolint:containedctx // needed to tell our deadline from the caller's cancellation
	cancel    context.CancelFunc
}

func (b *timeoutBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		return n, b.transport.classify(b.ctx, b.parent, b.req, err)
	}

	return n, err //nolint:wrapcheck // io.EOF must reach the reader unwrapped
}

func (b *timeoutBody) Close() error {
	err := b.ReadCloser.Close()

	b.cancel()

	return err //nolint:wrapcheck // plain pass-through of the underlying Close
}

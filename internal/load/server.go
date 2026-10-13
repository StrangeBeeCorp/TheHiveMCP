package load

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const (
	// ready is how long the server may take to listen.
	ready = 30 * time.Second
	// stopWait is how long the server may take to stop on an interrupt.
	stopWait = 5 * time.Second
	// dialEvery spaces the tries to reach the server while it starts.
	dialEvery = 100 * time.Millisecond
)

// Build builds the server of the module at root into dir, and returns the binary's path.
// It skips VCS stamping: in the pinned image the checkout belongs to another user, so git
// refuses to read it.
func Build(ctx context.Context, root, dir string) (string, error) {
	bin := filepath.Join(dir, "thehivemcp")
	cmd := exec.CommandContext(ctx, "go", "build", "-buildvcs=false", "-o", bin, "./cmd/server") // #nosec G204 -- the module's own server, into the run's directory
	cmd.Dir = root

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go build ./cmd/server: %w\n%s", err, out)
	}

	return bin, nil
}

// server is the running server and its logs.
type server struct {
	url  string
	cmd  *exec.Cmd
	logs *syncBuffer
	done chan struct{}
}

func (s server) pid() int { return s.cmd.Process.Pid }

// stop interrupts the server, and kills it if it does not stop.
func (s server) stop() {
	_ = s.cmd.Process.Signal(os.Interrupt)

	select {
	case <-s.done:
	case <-time.After(stopWait):
		_ = s.cmd.Process.Kill()
		<-s.done
	}
}

// start runs the server as its HTTP users do, on a free local port, with no credentials of its
// own (the clients send theirs), and waits until it listens.
func start(ctx context.Context, bin, theHiveURL string) (server, error) {
	port, err := freePort(ctx)
	if err != nil {
		return server{}, err
	}

	logs := &syncBuffer{}
	cmd := exec.CommandContext(ctx, bin, "--transport", "http") // #nosec G204 -- the server this run built

	cmd.Env = append(os.Environ(),
		"THEHIVE_URL="+theHiveURL, "MCP_BIND_HOST=127.0.0.1", "MCP_PORT="+strconv.Itoa(port), "LOG_LEVEL=warn",
		"THEHIVE_API_KEY=", "THEHIVE_USERNAME=", "THEHIVE_PASSWORD=", "THEHIVE_ORGANISATION=",
	)
	cmd.Stdout, cmd.Stderr = logs, logs

	err = cmd.Start()
	if err != nil {
		return server{}, fmt.Errorf("start the server: %w", err)
	}

	srv := server{url: fmt.Sprintf("http://127.0.0.1:%d/mcp", port), cmd: cmd, logs: logs, done: make(chan struct{})}

	go func() { _ = cmd.Wait(); close(srv.done) }()

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	deadline := time.Now().Add(ready)

	var dialer net.Dialer

	for time.Now().Before(deadline) {
		select {
		case <-srv.done:
			return server{}, fmt.Errorf("%w:\n%s", errExited, logs.String())
		default:
		}

		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err == nil {
			_ = conn.Close()

			return srv, nil
		}

		time.Sleep(dialEvery)
	}

	srv.stop()

	return server{}, fmt.Errorf("%w within %s:\n%s", errNotListening, ready, logs.String())
}

var (
	errExited       = errors.New("the server exited as it started")
	errNotListening = errors.New("the server did not listen")
	errNotTCP       = errors.New("a free port: not a TCP address")
)

func freePort(ctx context.Context) (int, error) {
	var lc net.ListenConfig

	l, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("a free port: %w", err)
	}
	defer func() { _ = l.Close() }()

	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errNotTCP
	}

	return addr.Port, nil
}

// syncBuffer is the server's output, written by its process and read on a failure.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	n, err := s.b.Write(p)
	if err != nil {
		return n, fmt.Errorf("server log: %w", err)
	}

	return n, nil
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.b.String()
}

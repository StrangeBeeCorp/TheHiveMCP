//go:build unix

package hooks

import (
	"fmt"
	"os"
	"syscall"
)

// lock takes an exclusive flock on path and returns its release.
func lock(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, filePerm) // #nosec G304 -- the lock file of this checkout's state
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}

	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX) // #nosec G115 -- a file descriptor fits an int
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}

	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) // #nosec G115 -- a file descriptor fits an int
		_ = f.Close()
	}, nil
}

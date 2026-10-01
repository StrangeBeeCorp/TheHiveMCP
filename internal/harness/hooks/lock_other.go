//go:build !unix

package hooks

// lock is a no-op where flock does not exist: the harness runs on macOS and Linux.
func lock(string) (func(), error) {
	return func() {}, nil
}

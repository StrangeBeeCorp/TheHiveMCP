package evidence

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// CoverageProfile is where the coverage gate writes the unit suite's profile, at the root.
const CoverageProfile = "coverage.out"

func profilePath(root, head string) string {
	return filepath.Join(root, Dir, head+".coverage.out")
}

// ClearProfile removes the coverage profile an earlier run left, before make pr's gates run: the
// profile KeepProfile then finds is this run's, or there is none.
func ClearProfile(root string) error {
	err := os.Remove(filepath.Join(root, CoverageProfile))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove the last run's %s: %w", CoverageProfile, err)
	}

	return nil
}

// KeepProfile keeps the coverage profile of a make pr run beside its evidence, under its commit:
// the pull request's change metrics read which changed statements the tests ran. It reports
// whether there was one.
func KeepProfile(root, head string) (bool, error) {
	data, err := os.ReadFile(filepath.Join(root, CoverageProfile)) // #nosec G304 -- this checkout's own profile, at a fixed path
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("read %s: %w", CoverageProfile, err)
	}

	dst := profilePath(root, head)

	err = os.MkdirAll(filepath.Dir(dst), dirPerm)
	if err == nil {
		err = os.WriteFile(dst, data, filePerm) // #nosec G703 -- this checkout's own record, named by a commit id
	}

	if err != nil {
		return false, fmt.Errorf("keep the coverage profile: %w", err)
	}

	return true, nil
}

// Profile is the coverage profile make pr kept for a commit, or nil when it kept none.
func Profile(root, head string) ([]byte, error) {
	data, err := os.ReadFile(profilePath(root, head)) // #nosec G304 -- this checkout's own record, named by a commit id
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("read the coverage profile of %s: %w", short(head), err)
	}

	return data, nil
}

package checks

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Tree is the set of files a check reads: the working tree, or the index when the check judges
// a commit, so that a commit is judged on exactly what it carries.
type Tree interface {
	// Files lists the files, sorted.
	Files() []string
	// Read returns a file's content, or an error wrapping fs.ErrNotExist.
	Read(name string) ([]byte, error)
	// Exists reports whether the path is a file or a directory with files in it.
	Exists(name string) bool
}

// DirTree is the working tree: the listed files, read from disk under Root.
type DirTree struct {
	Root  string
	Names []string
}

// Files lists the files.
func (d DirTree) Files() []string { return d.Names }

// Read reads a file from disk.
func (d DirTree) Read(name string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(d.Root, name)) // #nosec G304 -- names come from git ls-files
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}

	return data, nil
}

// Exists stats the path on disk.
func (d DirTree) Exists(name string) bool {
	_, err := os.Stat(filepath.Join(d.Root, name)) // #nosec G703 -- only tells whether a path a document names exists
	return err == nil
}

// MemTree holds the files in memory: the index for a commit, or a test's fixture.
type MemTree map[string][]byte

// Files lists the files, sorted.
func (m MemTree) Files() []string {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}

	sort.Strings(names)

	return names
}

// Read returns a file's content.
func (m MemTree) Read(name string) ([]byte, error) {
	data, ok := m[name]
	if !ok {
		return nil, fmt.Errorf("read %s: %w", name, fs.ErrNotExist)
	}

	return data, nil
}

// Exists reports whether the name is a file, or a directory holding one.
func (m MemTree) Exists(name string) bool {
	name = strings.TrimSuffix(name, "/")
	if _, ok := m[name]; ok {
		return true
	}

	for n := range m {
		if strings.HasPrefix(n, name+"/") {
			return true
		}
	}

	return false
}

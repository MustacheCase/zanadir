// Package repofs reads and writes the files of the repository under scan,
// refusing any path that leaves it, whether through .. or a symlink.
package repofs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ReadFile reads path.
func ReadFile(root, path string) ([]byte, error) {
	r, rel, err := open(root, path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	return r.ReadFile(rel)
}

// WriteFile writes path, creating its directory.
func WriteFile(root, path string, data []byte, perm fs.FileMode) error {
	r, rel, err := open(root, path)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	if err := r.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
		return err
	}
	return r.WriteFile(rel, data, perm)
}

// open takes path as built from root, so both are relative to the working
// directory or both absolute; anything else is refused.
func open(root, path string) (*os.Root, string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, "", fmt.Errorf("%s is outside the repository", path)
	}
	r, err := os.OpenRoot(root)
	return r, rel, err
}

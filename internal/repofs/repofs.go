// Package repofs reads and writes the files of the repository under scan,
// refusing any path that leaves it, whether through .. or a symlink.
package repofs

import (
	"errors"
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

// CreateOutput opens a file zanadir writes for the operator, such as a report,
// badge or baseline, refusing a symlink so a scanned repository cannot point
// the write elsewhere.
func CreateOutput(path string, perm fs.FileMode) (*os.File, error) {
	if info, err := os.Lstat(path); err == nil && info.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s is a symbolic link; refusing to write through it", path)
	}
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm) //nolint:gosec // operator-supplied path, symlinks refused above
}

// WriteOutput writes data to path as CreateOutput opens it.
func WriteOutput(path string, data []byte, perm fs.FileMode) error {
	f, err := CreateOutput(path, perm)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	return errors.Join(err, f.Close())
}

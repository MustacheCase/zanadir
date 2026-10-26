// Package repofs reads and writes the files of the repository under scan,
// refusing any path that leaves it, whether through .. or a symlink.
package repofs

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ReadFile reads path.
func ReadFile(root, path string) ([]byte, error) {
	return ReadFileLimit(root, path, maxReadBytes)
}

// maxReadBytes is the largest file ReadFile returns. A variable so tests can
// lower it.
var maxReadBytes int64 = 16 << 20

// ReadFileLimit reads a file of the repository, refusing one over limit bytes
// without reading the rest of it.
func ReadFileLimit(root, path string, limit int64) ([]byte, error) {
	r, rel, err := open(root, path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	f, err := r.Open(rel)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err == nil && int64(len(data)) > limit {
		err = fmt.Errorf("%s is larger than %d bytes", path, limit)
	}
	return data, err
}

// Exists reports whether path names anything inside the repository, without
// following a symlink out of it.
func Exists(root, path string) bool {
	r, rel, err := open(root, path)
	if err != nil {
		return false
	}
	defer func() { _ = r.Close() }()
	_, err = r.Lstat(rel)
	return err == nil
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

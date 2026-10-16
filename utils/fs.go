package utils

import (
	"os"
	"path/filepath"
)

// IsFile reports whether path exists and is a regular file or a link to one.
func IsFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// IsDir reports whether path exists and is a directory.
func IsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// FirstFile returns the first of names, relative to root, that is a file, or
// "" if none is.
func FirstFile(root string, names ...string) string {
	for _, name := range names {
		if IsFile(filepath.Join(root, name)) {
			return name
		}
	}
	return ""
}

// YAMLFiles lists the .yml and .yaml files directly inside dir, in name order.
// A missing directory has none.
func YAMLFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		switch filepath.Ext(entry.Name()) {
		case ".yml", ".yaml":
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	return files
}

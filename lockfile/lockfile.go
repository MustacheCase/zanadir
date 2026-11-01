// Package lockfile reads a repository's lockfiles for packages that no
// dependency scanner can look up.
package lockfile

import (
	"bufio"
	"bytes"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/MustacheCase/zanadir/internal/repofs"
)

// UV is uv's lockfile, at the repository root.
const UV = "uv.lock"

var (
	packageHeader = regexp.MustCompile(`^\[\[package\]\]\s*$`)
	packageName   = regexp.MustCompile(`^name = "([^"]+)"`)
	// A git or direct URL source has no name and version a vulnerability
	// database can match. A registry source can, even off pypi.org, since a
	// mirror or proxy serves PyPI's packages under their own names; path,
	// directory, editable and virtual sources are the repository's own code.
	unauditableSource = regexp.MustCompile(`^source = \{ (?:git|url) = `)
)

// Unauditable lists, sorted, the packages uv.lock installs from a git or URL
// source. A missing or unreadable lockfile lists none.
func Unauditable(dir string) []string {
	data, err := repofs.ReadFile(dir, filepath.Join(dir, UV))
	if err != nil {
		return nil
	}
	var found []string
	name := ""
	// uv writes one key per line, so the table needs no TOML parser.
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		switch {
		case packageHeader.Match(line):
			name = ""
		case name == "" && packageName.Match(line):
			name = string(packageName.FindSubmatch(line)[1])
		case name != "" && unauditableSource.Match(line):
			found = append(found, name)
		}
	}
	slices.Sort(found)
	return slices.Compact(found)
}

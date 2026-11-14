package lockfile

import (
	"cmp"
	"encoding/json"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/MustacheCase/zanadir/internal/repofs"
)

// NPM is npm's lockfile, at the repository root.
const NPM = "package-lock.json"

type npmLock struct {
	Packages     map[string]struct{ Resolved string } `json:"packages"`
	Dependencies map[string]npmDependency             `json:"dependencies"`
}

// npmDependency is lockfile v1's nested tree, which v2 also keeps.
type npmDependency struct {
	Version      string
	Resolved     string
	Dependencies map[string]npmDependency
}

var gitSpec = regexp.MustCompile(`^(?:git\+|git:|git@|github:|gitlab:|bitbucket:)`)

// UnauditableNPM lists, sorted, the packages package-lock.json installs from
// git or from a tarball URL that isn't a registry's. A missing or unreadable
// lockfile lists none.
func UnauditableNPM(dir string) []string {
	data, err := repofs.ReadFile(dir, filepath.Join(dir, NPM))
	if err != nil {
		return nil
	}
	var lock npmLock
	if json.Unmarshal(data, &lock) != nil {
		return nil
	}
	var found []string
	for key, entry := range lock.Packages {
		if i := strings.LastIndex(key, "node_modules/"); i >= 0 && unauditableNPM(entry.Resolved) {
			found = append(found, key[i+len("node_modules/"):])
		}
	}
	if lock.Packages == nil {
		found = walkNPM(lock.Dependencies, found)
	}
	slices.Sort(found)
	return slices.Compact(found)
}

func walkNPM(deps map[string]npmDependency, found []string) []string {
	for name, dep := range deps {
		if unauditableNPM(cmp.Or(dep.Resolved, dep.Version)) {
			found = append(found, name)
		}
		found = walkNPM(dep.Dependencies, found)
	}
	return found
}

// unauditableNPM reports whether a resolved source can't be looked up by name
// and version. Registry tarballs, npm's and its mirrors', sit under "/-/", and
// GitHub Packages serves its own; file: and link entries are first-party.
func unauditableNPM(resolved string) bool {
	if gitSpec.MatchString(resolved) {
		return true
	}
	u, err := url.Parse(resolved)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") &&
		!strings.Contains(u.Path, "/-/") && u.Host != "npm.pkg.github.com"
}

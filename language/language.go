// Package language detects the programming languages a repository uses.
package language

import (
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/MustacheCase/zanadir/internal/repofs"
	"github.com/MustacheCase/zanadir/utils"
)

// Canonical language names, matching the `language` field in suggestions.yaml.
const (
	Go         = "Go"
	Python     = "Python"
	JavaScript = "JavaScript"
	Ruby       = "Ruby"
	Java       = "Java"
	Rust       = "Rust"
	PHP        = "PHP"
	CSharp     = "C#"
)

// markers maps a dependency-manifest filename to the language it implies.
// Manifests are less ambiguous than counting source file suffixes.
var markers = map[string]string{
	"go.mod":           Go,
	"requirements.txt": Python,
	"pyproject.toml":   Python,
	"setup.py":         Python,
	"Pipfile":          Python,
	"package.json":     JavaScript,
	"Gemfile":          Ruby,
	"pom.xml":          Java,
	"build.gradle":     Java,
	"build.gradle.kts": Java,
	"Cargo.toml":       Rust,
	"composer.json":    PHP,
}

// suffixMarkers covers languages identified by extension, not a fixed filename.
var suffixMarkers = map[string]string{
	".gemspec": Ruby,
	".csproj":  CSharp,
	".sln":     CSharp,
}

// sourceSuffixes maps a source file extension to its language. Detect falls
// back to them only when no manifest names a language.
var sourceSuffixes = map[string]string{
	".py":   Python,
	".js":   JavaScript,
	".ts":   JavaScript,
	".go":   Go,
	".rb":   Ruby,
	".java": Java,
	".rs":   Rust,
	".php":  PHP,
	".cs":   CSharp,
}

// Names returns the sorted canonical language names recognised by Detect.
func Names() []string {
	found := make(map[string]bool)
	for _, name := range markers {
		found[name] = true
	}
	for _, name := range suffixMarkers {
		found[name] = true
	}
	return slices.Sorted(maps.Keys(found))
}

// Detect returns the languages a repository appears to be written in, sorted.
// It inspects the root and its immediate subdirectories, covering the common
// monorepo layout without walking the whole tree. With no manifest it falls
// back to source file extensions at the same depth.
func Detect(dir string) []string {
	if found := detect(dir, func(dir string, found map[string]bool) {
		collect(dir, found, func(string, string) bool { return true })
	}); len(found) > 0 {
		return found
	}
	return detect(dir, sources)
}

// WithDependencies is Detect for dependency scanning: a pyproject.toml that
// declares no dependencies (only a ruff config, say) names nothing to scan.
// It has no source file fallback: a .py file declares no dependencies.
func WithDependencies(dir string) []string {
	return detect(dir, func(dir string, found map[string]bool) {
		collect(dir, found, declaresDependencies)
	})
}

func detect(dir string, collect func(dir string, found map[string]bool)) []string {
	found := make(map[string]bool)

	collect(dir, found)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return slices.Sorted(maps.Keys(found))
	}
	for _, entry := range entries {
		if !entry.IsDir() || utils.IsDependencyDir(entry.Name()) || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		collect(filepath.Join(dir, entry.Name()), found)
	}

	return slices.Sorted(maps.Keys(found))
}

func collect(dir string, found map[string]bool, counts func(dir, name string) bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if lang, ok := markers[name]; ok {
			if counts(dir, name) {
				found[lang] = true
			}
			continue
		}
		for suffix, lang := range suffixMarkers {
			if strings.HasSuffix(name, suffix) {
				found[lang] = true
				break
			}
		}
	}
}

// sources records the languages of the source files directly in dir,
// skipping dotfiles such as .eslintrc.js.
func sources(dir string, found map[string]bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if lang, ok := sourceSuffixes[filepath.Ext(entry.Name())]; ok {
			found[lang] = true
		}
	}
}

var (
	comment         = regexp.MustCompile(`(?m)#.*$`)
	dependencyTable = regexp.MustCompile(`(?m)^\s*\[(?:project\.optional-dependencies|dependency-groups|tool\.poetry(?:\.group\.[^\]]+)?\.(?:dev-)?dependencies|tool\.pdm\.dev-dependencies)\]`)
	dependencyList  = regexp.MustCompile(`(?m)^\s*(?:dependencies|dev-dependencies)\s*=\s*\[\s*[^\]\s]`)
	dynamicList     = regexp.MustCompile(`(?m)^\s*dynamic\s*=\s*\[[^\]]*["']dependencies["']`)
)

// declaresDependencies reports whether a pyproject.toml lists any dependency;
// any other manifest, or one it cannot read, counts.
func declaresDependencies(dir, name string) bool {
	if name != "pyproject.toml" {
		return true
	}
	data, err := repofs.ReadFile(dir, filepath.Join(dir, name))
	if err != nil {
		return true
	}
	text := comment.ReplaceAll(data, nil)
	return dependencyTable.Match(text) || dependencyList.Match(text) || dynamicList.Match(text)
}

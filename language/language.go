package language

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
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

// skipDirs hold dependencies rather than the project's own code, so their
// manifests say nothing about what the repository is written in.
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"testdata":     true,
	"third_party":  true,
}

// Detect returns the languages a repository appears to be written in, sorted.
// It inspects the root and its immediate subdirectories, covering the common
// monorepo layout without walking the whole tree.
func Detect(dir string) []string {
	found := make(map[string]bool)

	collect(dir, found)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return sortedKeys(found)
	}
	for _, entry := range entries {
		if !entry.IsDir() || skipDirs[entry.Name()] || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		collect(filepath.Join(dir, entry.Name()), found)
	}

	return sortedKeys(found)
}

func collect(dir string, found map[string]bool) {
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
			found[lang] = true
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

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

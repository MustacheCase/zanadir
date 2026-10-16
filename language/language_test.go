package language_test

import (
	"path/filepath"
	"testing"

	"github.com/MustacheCase/zanadir/internal/testrepo"
	"github.com/MustacheCase/zanadir/language"
	"github.com/stretchr/testify/assert"
)

func TestDetectByManifest(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected []string
	}{
		{name: "go", files: []string{"go.mod"}, expected: []string{language.Go}},
		{name: "python requirements", files: []string{"requirements.txt"}, expected: []string{language.Python}},
		{name: "python pyproject", files: []string{"pyproject.toml"}, expected: []string{language.Python}},
		{name: "javascript", files: []string{"package.json"}, expected: []string{language.JavaScript}},
		{name: "ruby gemfile", files: []string{"Gemfile"}, expected: []string{language.Ruby}},
		{name: "ruby gemspec suffix", files: []string{"mygem.gemspec"}, expected: []string{language.Ruby}},
		{name: "java maven", files: []string{"pom.xml"}, expected: []string{language.Java}},
		{name: "java gradle", files: []string{"build.gradle"}, expected: []string{language.Java}},
		{name: "rust", files: []string{"Cargo.toml"}, expected: []string{language.Rust}},
		{name: "php", files: []string{"composer.json"}, expected: []string{language.PHP}},
		{name: "csharp suffix", files: []string{"App.csproj"}, expected: []string{language.CSharp}},
		{name: "none", files: []string{"README.md"}, expected: nil},
		{
			name:  "polyglot is sorted",
			files: []string{"go.mod", "package.json", "requirements.txt"},
			// sorted: Go, JavaScript, Python
			expected: []string{language.Go, language.JavaScript, language.Python},
		},
		{
			name:     "duplicate markers collapse to one language",
			files:    []string{"requirements.txt", "setup.py", "Pipfile"},
			expected: []string{language.Python},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := testrepo.Touch(t, tt.files...)
			assert.Equal(t, tt.expected, language.Detect(dir))
		})
	}
}

// Monorepos keep their manifests one level down.
func TestDetectLooksOneLevelDown(t *testing.T) {
	dir := testrepo.Touch(t, "services/api/go.mod", "services/web/package.json")

	// services/*/ is two levels down and intentionally not reached.
	assert.Empty(t, language.Detect(dir))

	flat := testrepo.Touch(t, "api/go.mod", "web/package.json")
	assert.Equal(t, []string{language.Go, language.JavaScript}, language.Detect(flat))
}

// Dependency directories carry manifests for code the project did not write.
func TestDetectSkipsDependencyDirectories(t *testing.T) {
	dir := testrepo.Touch(t, "go.mod", "node_modules/left-pad/package.json", "vendor/x/composer.json")

	assert.Equal(t, []string{language.Go}, language.Detect(dir))
}

func TestDetectSkipsHiddenDirectories(t *testing.T) {
	dir := testrepo.Touch(t, ".github/package.json", "go.mod")

	assert.Equal(t, []string{language.Go}, language.Detect(dir))
}

func TestDetectMissingDirectory(t *testing.T) {
	assert.Empty(t, language.Detect(filepath.Join(t.TempDir(), "nope")))
}
